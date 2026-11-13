// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package webtests

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/files"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/modules/github"
	"code.vikunja.io/api/pkg/modules/keyvalue"
	"code.vikunja.io/api/pkg/routes"
	"code.vikunja.io/api/pkg/user"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const githubTestSecret = "webhook-test-secret"

// fakeGithubAPI mimics the GitHub endpoints the integration calls, for Webpart-ro/test-repo
// (linked to project 1, identifier TEST1, in the fixtures).
type fakeGithubAPI struct {
	mu         sync.Mutex
	createdRef map[string]string
}

func (f *fakeGithubAPI) handler() http.Handler {
	mux := http.NewServeMux()
	base := "/repos/Webpart-ro/test-repo"
	mux.HandleFunc("GET "+base+"/installation", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id": 1}`)
	})
	mux.HandleFunc("POST /app/installations/1/access_tokens", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "t", "expires_at": time.Now().Add(time.Hour)})
	})
	mux.HandleFunc("GET "+base+"/branches", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"name":"develop","commit":{"sha":"d0"}},{"name":"TEST1-1-thing","commit":{"sha":"b1"}}]`)
	})
	mux.HandleFunc("GET "+base+"/pulls", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"number":3,"title":"TEST1-1 fix the thing","html_url":"https://github.com/Webpart-ro/test-repo/pull/3","state":"open","updated_at":"2026-11-13T10:00:00Z","user":{"login":"radu"},"head":{"ref":"TEST1-1-thing","sha":"b1"},"base":{"ref":"develop"}}]`)
	})
	mux.HandleFunc("GET "+base+"/commits/{sha}/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"total_count":1,"check_runs":[{"status":"completed","conclusion":"success","html_url":"https://ci/run/1"}]}`)
	})
	mux.HandleFunc("GET "+base+"/commits/{sha}/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("sha") == "num1" {
			_, _ = io.WriteString(w, `[]`)
			return
		}
		_, _ = io.WriteString(w, `[{"number":3,"title":"TEST1-1 fix the thing","html_url":"https://github.com/Webpart-ro/test-repo/pull/3","state":"closed","merged_at":"2026-11-13T10:00:00Z","updated_at":"2026-11-13T10:00:00Z","user":{"login":"radu"},"head":{"ref":"TEST1-1-thing","sha":"b1"},"base":{"ref":"develop"}}]`)
	})
	mux.HandleFunc("GET "+base+"/commits/{sha}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("sha") == "num1" {
			_, _ = io.WriteString(w, `{"commit":{"message":"chore: tidy"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"commit":{"message":"Merge pull request #3"}}`)
	})
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"default_branch":"main"}`)
	})
	mux.HandleFunc("GET "+base+"/git/ref/heads/{branch...}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("branch") == "develop" {
			_, _ = io.WriteString(w, `{"object":{"sha":"d0"}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("POST "+base+"/git/refs", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.createdRef[body["ref"]] = body["sha"]
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	})
	return mux
}

// setupGithubTestEnv is setupTestEnv with the GitHub integration switched on before the
// routes register (they are gated by github.enabled at registration time).
func setupGithubTestEnv(t *testing.T) (*echo.Echo, *fakeGithubAPI) {
	config.InitDefaultConfig()
	config.ServicePublicURL.Set("https://localhost")
	log.InitLogger()
	files.InitTests()
	user.InitTests()
	models.SetupTests()
	events.Fake()
	keyvalue.InitStorage()
	require.NoError(t, db.LoadFixtures())

	config.GithubEnabled.Set(true)
	config.GithubAppID.Set("1")
	config.GithubPrivateKeyPath.Set("unused-in-tests")
	config.GithubWebhookSecret.Set(githubTestSecret)
	config.GithubBotUsername.Set("user1")

	fake := &fakeGithubAPI{createdRef: map[string]string{}}
	server := httptest.NewServer(fake.handler())
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	github.SetDefaultForTests(github.NewClient(server.URL, "1", key, server.Client()))
	t.Cleanup(func() {
		server.Close()
		github.SetDefaultForTests(nil)
		config.GithubEnabled.Set(false)
	})

	e := routes.NewEcho()
	routes.RegisterRoutes(e)
	return e, fake
}

func TestGithubProjectRepos(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/projects/1/github/repos", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"repo":"test-repo"`)
	})
	t.Run("create, duplicate and invalid", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		token := humaTokenFor(t, &testuser1)
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/github/repos", `{"owner":"Webpart-ro","repo":"shop.git"}`, token, "")
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"repo":"shop"`, ".git is stripped")

		rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/github/repos", `{"owner":"webpart-ro","repo":"SHOP"}`, token, "")
		assert.Equal(t, http.StatusConflict, rec.Code, "duplicates are case-insensitive")

		rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/github/repos", `{"owner":"bad owner","repo":"x"}`, token, "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("read-only project member can list but not link", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		token := humaTokenFor(t, &testuser1)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/projects/9/github/repos", "", token, "")
		assert.Equal(t, http.StatusOK, rec.Code)
		rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/9/github/repos", `{"owner":"Webpart-ro","repo":"x"}`, token, "")
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
	t.Run("delete", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		token := humaTokenFor(t, &testuser1)
		rec := humaRequest(t, e, http.MethodDelete, "/api/v2/projects/9/github/repos/1", "", token, "")
		assert.Equal(t, http.StatusForbidden, rec.Code, "the link belongs to project 1, not 9")
		rec = humaRequest(t, e, http.MethodDelete, "/api/v2/projects/1/github/repos/1", "", token, "")
		assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	})
}

func TestGithubTaskDevelopment(t *testing.T) {
	t.Run("branches, pull requests and CI that mention the task", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/tasks/1/development", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		body := rec.Body.String()
		assert.Contains(t, body, `"key":"TEST1-1"`)
		assert.Contains(t, body, `"name":"TEST1-1-thing"`)
		assert.NotContains(t, body, `"name":"develop"`)
		assert.Contains(t, body, `"number":3`)
		assert.Contains(t, body, `"state":"success"`)
	})
	t.Run("a project without linked repositories returns nothing", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/tasks/18/development", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"repositories":[]`)
	})
	t.Run("no access to the task", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/tasks/1/development", "", humaTokenFor(t, &testuser15), "")
		assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, rec.Code)
	})
}

func TestGithubCreateBranch(t *testing.T) {
	t.Run("creates the branch from develop", func(t *testing.T) {
		e, fake := setupGithubTestEnv(t)
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/1/development/branches", `{"repo":"Webpart-ro/test-repo","name":"TEST1-1-new-work"}`, humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		assert.Equal(t, "d0", fake.createdRef["refs/heads/TEST1-1-new-work"])
	})
	t.Run("the name must mention the task", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/1/development/branches", `{"repo":"Webpart-ro/test-repo","name":"random-work"}`, humaTokenFor(t, &testuser1), "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("only linked repositories", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/1/development/branches", `{"repo":"someone/else","name":"TEST1-1-x"}`, humaTokenFor(t, &testuser1), "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("read-only users can't create branches", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/18/development/branches", `{"repo":"Webpart-ro/test-repo","name":"TEST9-1-x"}`, humaTokenFor(t, &testuser1), "")
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func signGithubPayload(body string) string {
	mac := hmac.New(sha256.New, []byte(githubTestSecret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func githubWebhookRequest(e *echo.Echo, event, body, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v2/integrations/github/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-Hub-Signature-256", signature)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

const deploymentPayload = `{
	"deployment_status": {"state": "%STATE%", "environment": "staging", "log_url": "https://github.com/Webpart-ro/test-repo/actions/runs/9"},
	"deployment": {"id": 4242, "ref": "develop", "sha": "abc123", "environment": "staging"},
	"repository": {"name": "test-repo", "owner": {"login": "Webpart-ro"}}
}`

func TestGithubWebhook(t *testing.T) {
	commentsOnTask1 := func(t *testing.T) int64 {
		s := db.NewSession()
		defer s.Close()
		n, err := s.Where("task_id = ? AND comment LIKE ?", 1, "%Deployed to <strong>staging</strong>%").Count(&models.TaskComment{})
		require.NoError(t, err)
		return n
	}

	t.Run("rejects a bad signature", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		body := strings.ReplaceAll(deploymentPayload, "%STATE%", "success")
		rec := githubWebhookRequest(e, "deployment_status", body, "sha256=00")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("comments once per deployment on the tasks it covers", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		body := strings.ReplaceAll(deploymentPayload, "%STATE%", "success")
		rec := githubWebhookRequest(e, "deployment_status", body, signGithubPayload(body))
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"task_ids":[1]`, "TEST1-1 is found through the merged pull request")
		assert.Equal(t, int64(1), commentsOnTask1(t))

		// GitHub redelivers: no second comment.
		rec = githubWebhookRequest(e, "deployment_status", body, signGithubPayload(body))
		require.Equal(t, http.StatusAccepted, rec.Code)
		assert.Equal(t, int64(1), commentsOnTask1(t))
	})
	t.Run("a bare number in the deployed branch matches the task", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		body := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(deploymentPayload, "%STATE%", "success"), `"ref": "develop"`, `"ref": "feat/1-something"`), `"sha": "abc123"`, `"sha": "num1"`)
		rec := githubWebhookRequest(e, "deployment_status", body, signGithubPayload(body))
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"task_ids":[1]`)
	})
	t.Run("ignores unsuccessful deployments and other events", func(t *testing.T) {
		e, _ := setupGithubTestEnv(t)
		body := strings.ReplaceAll(deploymentPayload, "%STATE%", "failure")
		rec := githubWebhookRequest(e, "deployment_status", body, signGithubPayload(body))
		require.Equal(t, http.StatusAccepted, rec.Code)
		assert.Contains(t, rec.Body.String(), `"handled":false`)

		rec = githubWebhookRequest(e, "push", `{}`, signGithubPayload(`{}`))
		assert.Equal(t, http.StatusAccepted, rec.Code)
		assert.Equal(t, int64(0), commentsOnTask1(t))
	})
}

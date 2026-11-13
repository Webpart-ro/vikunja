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

package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGitHub serves the handful of endpoints the client uses for repo "acme/shop".
type fakeGitHub struct {
	t             *testing.T
	key           *rsa.PrivateKey
	tokenRequests atomic.Int32
	createdRef    map[string]string
	hasDevelop    bool
}

func (f *fakeGitHub) handler() http.Handler {
	mux := http.NewServeMux()
	appAuth := func(r *http.Request) bool {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		token, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return &f.key.PublicKey, nil })
		return err == nil && token.Valid
	}
	instAuth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "token inst-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /repos/acme/shop/installation", func(w http.ResponseWriter, r *http.Request) {
		if !appAuth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"id": 7}`)
	})
	mux.HandleFunc("GET /repos/acme/other/installation", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("POST /app/installations/7/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		if !appAuth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.tokenRequests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "inst-token", "expires_at": time.Now().Add(time.Hour)})
	})
	mux.HandleFunc("GET /repos/acme/shop/branches", func(w http.ResponseWriter, r *http.Request) {
		if !instAuth(w, r) {
			return
		}
		_, _ = io.WriteString(w, `[
			{"name":"develop","commit":{"sha":"d0"}},
			{"name":"HS-334-release-day","commit":{"sha":"b1"}},
			{"name":"feature/hs-334-footer","commit":{"sha":"b2"}},
			{"name":"HS-3345-other","commit":{"sha":"b3"}},
			{"name":"feat/334-numbered","commit":{"sha":"b4"}},
			{"name":"feat/3340-other","commit":{"sha":"b5"}}
		]`)
	})
	mux.HandleFunc("GET /repos/acme/shop/pulls", func(w http.ResponseWriter, r *http.Request) {
		if !instAuth(w, r) {
			return
		}
		_, _ = io.WriteString(w, `[
			{"number":12,"title":"HS-334: release day emails","html_url":"https://github.com/acme/shop/pull/12","state":"open","draft":false,"merged_at":null,"updated_at":"2026-11-10T10:00:00Z","user":{"login":"radu"},"head":{"ref":"HS-334-release-day","sha":"b1"},"base":{"ref":"develop"}},
			{"number":9,"title":"Old work","body":"Part of HS-334","html_url":"https://github.com/acme/shop/pull/9","state":"closed","merged_at":"2026-11-01T10:00:00Z","updated_at":"2026-11-01T10:00:00Z","user":{"login":"ana"},"head":{"ref":"old","sha":"o1"},"base":{"ref":"develop"}},
			{"number":7,"title":"Footer tweaks","body":"Closes #334","html_url":"https://github.com/acme/shop/pull/7","state":"closed","merged_at":null,"updated_at":"2026-10-01T10:00:00Z","user":{"login":"ana"},"head":{"ref":"tweaks","sha":"t1"},"base":{"ref":"develop"}},
			{"number":6,"title":"Other","body":"See acme/other#334","html_url":"https://github.com/acme/shop/pull/6","state":"closed","merged_at":null,"updated_at":"2026-10-01T10:00:00Z","user":{"login":"ana"},"head":{"ref":"x6","sha":"x6"},"base":{"ref":"develop"}},
			{"number":5,"title":"HS-3345 unrelated","html_url":"https://github.com/acme/shop/pull/5","state":"open","updated_at":"2026-11-12T10:00:00Z","user":{"login":"x"},"head":{"ref":"x","sha":"x1"},"base":{"ref":"develop"}}
		]`)
	})
	mux.HandleFunc("GET /repos/acme/shop/commits/{sha}/check-runs", func(w http.ResponseWriter, r *http.Request) {
		if !instAuth(w, r) {
			return
		}
		switch r.PathValue("sha") {
		case "b1":
			_, _ = io.WriteString(w, `{"total_count":2,"check_runs":[{"status":"completed","conclusion":"success","html_url":"https://ci/ok"},{"status":"completed","conclusion":"failure","html_url":"https://ci/fail"}]}`)
		case "b2":
			_, _ = io.WriteString(w, `{"total_count":1,"check_runs":[{"status":"in_progress","conclusion":null,"html_url":"https://ci/run"}]}`)
		default:
			_, _ = io.WriteString(w, `{"total_count":0,"check_runs":[]}`)
		}
	})
	mux.HandleFunc("GET /repos/acme/shop", func(w http.ResponseWriter, r *http.Request) {
		if !instAuth(w, r) {
			return
		}
		_, _ = io.WriteString(w, `{"default_branch":"main"}`)
	})
	mux.HandleFunc("GET /repos/acme/shop/git/ref/heads/{branch...}", func(w http.ResponseWriter, r *http.Request) {
		if !instAuth(w, r) {
			return
		}
		switch r.PathValue("branch") {
		case "main":
			_, _ = io.WriteString(w, `{"object":{"sha":"m0"}}`)
		case "develop":
			if f.hasDevelop {
				_, _ = io.WriteString(w, `{"object":{"sha":"d0"}}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("POST /repos/acme/shop/git/refs", func(w http.ResponseWriter, r *http.Request) {
		if !instAuth(w, r) {
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body["ref"] == "refs/heads/exists" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"message":"Reference already exists"}`)
			return
		}
		f.createdRef[body["ref"]] = body["sha"]
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	})
	return mux
}

func newTestClient(t *testing.T, hasDevelop bool) (*Client, *fakeGitHub) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	fake := &fakeGitHub{t: t, key: key, createdRef: map[string]string{}, hasDevelop: hasDevelop}
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)
	return NewClient(server.URL, "123", key, server.Client()), fake
}

func TestMentionsKey(t *testing.T) {
	assert.True(t, MentionsKey("HS-334-release-day", "HS-334"))
	assert.True(t, MentionsKey("feature/hs-334-footer", "HS-334"))
	assert.True(t, MentionsKey("Fix (HS-334)", "HS-334"))
	assert.False(t, MentionsKey("HS-3345-other", "HS-334"))
	assert.False(t, MentionsKey("XHS-334", "HS-334"))
	assert.False(t, MentionsKey("anything", "#12"), "tasks without a project identifier never match")
}

func TestNumberReferences(t *testing.T) {
	assert.True(t, RefMentionsNumber("feat/335-checkout", 335))
	assert.True(t, RefMentionsNumber("335", 335))
	assert.True(t, RefMentionsNumber("fix/335/part-2", 335))
	assert.False(t, RefMentionsNumber("feat/3350-x", 335))
	assert.False(t, RefMentionsNumber("v2-335", 335))
	assert.False(t, RefMentionsNumber("feat/335-x", 0))

	assert.True(t, BodyMentionsNumber("Closes #335", 335))
	assert.True(t, BodyMentionsNumber("Closes #138. Refs #335. This PR", 335))
	assert.True(t, BodyMentionsNumber("(part of #335).", 335))
	assert.True(t, BodyMentionsNumber("fixes: #335", 335))
	assert.False(t, BodyMentionsNumber("the supplier feeds (#335) will take over", 335), "passing mentions don't count")
	assert.False(t, BodyMentionsNumber("groundwork for #335", 335))
	assert.False(t, BodyMentionsNumber("Closes other/repo#335", 335))
	assert.False(t, BodyMentionsNumber("Closes #3350", 335))

	assert.True(t, TitleMentionsNumber("feat(feeds): supplier feed checks (#335)", 335))
	assert.False(t, TitleMentionsNumber("fix (#3350)", 335))

	assert.Equal(t, []int64{107}, NumbersInRef("feat/107-supplier-feeds"))
	assert.Equal(t, []int64{}, NumbersInRef("develop"))
	assert.Equal(t, int64(334), numberFromKey("HS-334"))
}

func TestKeysInText(t *testing.T) {
	assert.Equal(t, []string{"HS-334", "BOC-30"}, KeysInText("Merge branch 'hs-334-x' (BOC-30, HS-334)"))
	assert.Empty(t, KeysInText("no keys here, v2-final"))
}

func TestDevelopment(t *testing.T) {
	client, fake := newTestClient(t, true)
	dev, err := client.Development(context.Background(), "acme", "shop", "HS-334")
	require.NoError(t, err)

	require.True(t, dev.Accessible)
	names := []string{}
	for _, b := range dev.Branches {
		names = append(names, b.Name)
	}
	assert.Equal(t, []string{"HS-334-release-day", "feature/hs-334-footer", "feat/334-numbered"}, names, "bare-number branches match too")

	require.Len(t, dev.PullRequests, 3, "#7 matches through 'Closes #334'; #6's cross-repo ref doesn't")
	assert.Equal(t, int64(12), dev.PullRequests[0].Number, "open pull requests come first")
	assert.Equal(t, "open", dev.PullRequests[0].State)
	assert.Equal(t, "failure", dev.PullRequests[0].CI.State)
	assert.Equal(t, "https://ci/fail", dev.PullRequests[0].CI.URL, "the failing run is linked")
	assert.Equal(t, "merged", dev.PullRequests[1].State, "matched through the description")
	assert.Equal(t, int64(7), dev.PullRequests[2].Number)
	assert.Nil(t, dev.PullRequests[1].CI, "closed pull requests skip the CI lookup")

	assert.Equal(t, "pending", dev.Branches[1].CI.State)

	// A second call reuses the cached installation token.
	_, err = client.Development(context.Background(), "acme", "shop", "HS-334")
	require.NoError(t, err)
	assert.Equal(t, int32(1), fake.tokenRequests.Load())
}

func TestDevelopmentRepoWithoutApp(t *testing.T) {
	client, _ := newTestClient(t, true)
	dev, err := client.Development(context.Background(), "acme", "other", "HS-334")
	require.NoError(t, err)
	assert.False(t, dev.Accessible)
	assert.Empty(t, dev.Branches)
}

func TestCreateBranch(t *testing.T) {
	config.InitDefaultConfig() // github.defaultbasebranch = develop
	t.Run("from develop by default", func(t *testing.T) {
		client, fake := newTestClient(t, true)
		branch, err := client.CreateBranch(context.Background(), "acme", "shop", "", "HS-1-new")
		require.NoError(t, err)
		assert.Equal(t, "d0", fake.createdRef["refs/heads/HS-1-new"])
		assert.Equal(t, "https://github.com/acme/shop/tree/HS-1-new", branch.URL)
	})
	t.Run("falls back to the repository default branch without develop", func(t *testing.T) {
		client, fake := newTestClient(t, false)
		_, err := client.CreateBranch(context.Background(), "acme", "shop", "", "HS-1-new")
		require.NoError(t, err)
		assert.Equal(t, "m0", fake.createdRef["refs/heads/HS-1-new"])
	})
	t.Run("an explicit missing base is an error", func(t *testing.T) {
		client, _ := newTestClient(t, false)
		_, err := client.CreateBranch(context.Background(), "acme", "shop", "release", "HS-1-new")
		assert.ErrorIs(t, err, ErrBaseBranchNotFound)
	})
	t.Run("existing branch", func(t *testing.T) {
		client, _ := newTestClient(t, true)
		_, err := client.CreateBranch(context.Background(), "acme", "shop", "develop", "exists")
		assert.ErrorIs(t, err, ErrBranchExists)
	})
}

func TestValidBranchName(t *testing.T) {
	assert.True(t, ValidBranchName("HS-334-release-day"))
	assert.True(t, ValidBranchName("feature/HS-334-x"))
	for _, bad := range []string{"", "-x", "a..b", "a//b", "a/", "a.", "a.lock", "a b", "a~b", "a:b"} {
		assert.False(t, ValidBranchName(bad), bad)
	}
}

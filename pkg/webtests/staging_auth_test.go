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
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/files"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/modules/keyvalue"
	"code.vikunja.io/api/pkg/modules/stagingauth"
	"code.vikunja.io/api/pkg/routes"
	"code.vikunja.io/api/pkg/user"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const stagingBasicPassword = "ci-password"

// setupStagingAuthTestEnv switches staging SSO on before the routes register.
func setupStagingAuthTestEnv(t *testing.T) *echo.Echo {
	config.InitDefaultConfig()
	config.ServicePublicURL.Set("https://tasks.webpart.ro/")
	log.InitLogger()
	files.InitTests()
	user.InitTests()
	models.SetupTests()
	events.Fake()
	keyvalue.InitStorage()
	require.NoError(t, db.LoadFixtures())

	hash, err := bcrypt.GenerateFromPassword([]byte(stagingBasicPassword), bcrypt.MinCost)
	require.NoError(t, err)
	config.StagingAuthEnabled.Set(true)
	config.StagingAuthCookieDomain.Set("webpart.ro")
	config.StagingAuthBasicAuthUsers.Set([]string{"webpart:" + string(hash)})
	t.Cleanup(func() { config.StagingAuthEnabled.Set(false) })

	e := routes.NewEcho()
	routes.RegisterRoutes(e)
	return e
}

func stagingVerify(e *echo.Echo, method string, headers map[string]string, cookie string) *httptest.ResponseRecorder {
	return stagingVerifyAt(e, "/api/v2/staging-auth/verify", method, headers, cookie)
}

func stagingVerifyAt(e *echo.Echo, path, method string, headers map[string]string, cookie string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Forwarded-Host", "shop-staging.webpart.ro")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Uri", "/ro/cart?step=2")
	req.Header.Set("X-Forwarded-Method", method)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: stagingauth.CookieName, Value: cookie})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestStagingAuthVerify(t *testing.T) {
	e := setupStagingAuthTestEnv(t)

	t.Run("browser without a session is sent to the login page", func(t *testing.T) {
		rec := stagingVerify(e, http.MethodGet, nil, "")
		require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
		assert.Equal(t,
			"https://tasks.webpart.ro/staging-login?rd=https%3A%2F%2Fshop-staging.webpart.ro%2Fro%2Fcart%3Fstep%3D2",
			rec.Header().Get("Location"))
	})
	t.Run("curl without a session gets 401 (deploy health checks)", func(t *testing.T) {
		rec := stagingVerify(e, http.MethodGet, map[string]string{"Accept": "*/*"}, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("non-GET without a session is refused", func(t *testing.T) {
		rec := stagingVerify(e, http.MethodPost, nil, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("host outside the cookie domain is refused, not redirected", func(t *testing.T) {
		rec := stagingVerify(e, http.MethodGet, map[string]string{"X-Forwarded-Host": "evil.com"}, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("valid session passes", func(t *testing.T) {
		token, err := stagingauth.NewSessionToken(testuser1.ID, time.Now())
		require.NoError(t, err)
		rec := stagingVerify(e, http.MethodPost, nil, token)
		assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	})
	t.Run("session of an unknown user is not accepted", func(t *testing.T) {
		token, err := stagingauth.NewSessionToken(99999, time.Now())
		require.NoError(t, err)
		rec := stagingVerify(e, http.MethodGet, nil, token)
		assert.Equal(t, http.StatusFound, rec.Code)
	})
	t.Run("forged session is not accepted", func(t *testing.T) {
		rec := stagingVerify(e, http.MethodGet, nil, "not-a-token")
		assert.Equal(t, http.StatusFound, rec.Code)
	})
	t.Run("chat link unfurlers get the Webpart preview card, not the site", func(t *testing.T) {
		for _, ua := range []string{
			"Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)",
			"Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)",
			"WhatsApp/2.23.20.0",
			"facebookexternalhit/1.1 Facebot Twitterbot/1.0",
		} {
			rec := stagingVerify(e, http.MethodGet, map[string]string{"User-Agent": ua, "Accept": "*/*"}, "")
			require.Equal(t, http.StatusFound, rec.Code, ua)
			assert.Equal(t,
				"https://tasks.webpart.ro/api/v2/staging-auth/preview?rd=https%3A%2F%2Fshop-staging.webpart.ro%2Fro%2Fcart%3Fstep%3D2",
				rec.Header().Get("Location"), ua)
		}
	})
	t.Run("unfurler on a host outside the cookie domain is refused", func(t *testing.T) {
		rec := stagingVerify(e, http.MethodGet, map[string]string{"User-Agent": "Discordbot/2.0", "X-Forwarded-Host": "evil.com"}, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("basic credentials for automation", func(t *testing.T) {
		good := "Basic " + base64.StdEncoding.EncodeToString([]byte("webpart:"+stagingBasicPassword))
		bad := "Basic " + base64.StdEncoding.EncodeToString([]byte("webpart:nope"))
		assert.Equal(t, http.StatusOK, stagingVerify(e, http.MethodPost, map[string]string{"Authorization": good}, "").Code)
		assert.Equal(t, http.StatusUnauthorized, stagingVerify(e, http.MethodGet, map[string]string{"Authorization": bad}, "").Code)
	})
	t.Run("basic=0 refuses basic credentials but keeps sessions and the login redirect", func(t *testing.T) {
		const noBasic = "/api/v2/staging-auth/verify?basic=0"
		good := "Basic " + base64.StdEncoding.EncodeToString([]byte("webpart:"+stagingBasicPassword))
		admin := map[string]string{"X-Forwarded-Host": "admin.webpart.ro", "X-Forwarded-Uri": "/"}

		// Valid Basic, non-page request: refused.
		rec := stagingVerifyAt(e, noBasic, http.MethodPost, map[string]string{"Authorization": good}, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		// Valid Basic, browser page load: sent to the login page instead of let through.
		rec = stagingVerifyAt(e, noBasic, http.MethodGet, map[string]string{"Authorization": good, "X-Forwarded-Host": "admin.webpart.ro", "X-Forwarded-Uri": "/"}, "")
		require.Equal(t, http.StatusFound, rec.Code)
		assert.Equal(t, "https://tasks.webpart.ro/staging-login?rd=https%3A%2F%2Fadmin.webpart.ro%2F", rec.Header().Get("Location"))
		// curl without anything (deploy smoke test): 401.
		assert.Equal(t, http.StatusUnauthorized, stagingVerifyAt(e, noBasic, http.MethodGet, map[string]string{"Accept": "*/*"}, "").Code)
		// Session cookie still passes.
		token, err := stagingauth.NewSessionToken(testuser1.ID, time.Now())
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, stagingVerifyAt(e, noBasic, http.MethodGet, admin, token).Code)
		// basic=1 (or absent) keeps the old behaviour.
		assert.Equal(t, http.StatusOK, stagingVerifyAt(e, "/api/v2/staging-auth/verify?basic=1", http.MethodPost, map[string]string{"Authorization": good}, "").Code)
	})
}

func TestStagingAuthPreview(t *testing.T) {
	e := setupStagingAuthTestEnv(t)

	get := func(rd string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v2/staging-auth/preview?rd="+url.QueryEscape(rd), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	t.Run("renders Webpart Open Graph tags for a staging URL", func(t *testing.T) {
		rec := get("https://shop-staging.webpart.ro/ro/cart?step=2&x=<b>")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
		body := rec.Body.String()
		assert.Contains(t, body, `<meta property="og:site_name" content="Webpart">`)
		assert.Contains(t, body, `content="Webpart · staging · shop-staging.webpart.ro"`)
		assert.Contains(t, body, `<meta property="og:image" content="https://webpart.ro/og.png">`)
		assert.Contains(t, body, `noindex`)
		assert.NotContains(t, body, "<b>", "the URL must be escaped")
	})
	t.Run("refuses URLs outside the cookie domain", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, get("https://evil.com/").Code)
		assert.Equal(t, http.StatusBadRequest, get("http://shop-staging.webpart.ro/").Code)
	})
}

func TestStagingAuthSession(t *testing.T) {
	e := setupStagingAuthTestEnv(t)
	token := humaTokenFor(t, &testuser1)

	t.Run("sets the shared cookie and returns the target", func(t *testing.T) {
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/staging-auth/session",
			`{"redirect":"https://shop-staging.webpart.ro/ro/cart?step=2"}`, token, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"redirect":"https://shop-staging.webpart.ro/ro/cart?step=2"`)

		cookie := rec.Header().Get("Set-Cookie")
		assert.Contains(t, cookie, stagingauth.CookieName+"=")
		assert.Contains(t, cookie, "Domain=webpart.ro")
		assert.Contains(t, cookie, "HttpOnly")
		assert.Contains(t, cookie, "Secure")

		value := strings.TrimPrefix(strings.Split(cookie, ";")[0], stagingauth.CookieName+"=")
		id, err := stagingauth.ParseSessionToken(value)
		require.NoError(t, err)
		assert.Equal(t, testuser1.ID, id)
	})
	t.Run("redirects outside the domain are refused", func(t *testing.T) {
		for _, target := range []string{"https://evil.com/", "http://shop-staging.webpart.ro/", "https://webpart.ro.evil.com/"} {
			rec := humaRequest(t, e, http.MethodPost, "/api/v2/staging-auth/session", `{"redirect":"`+target+`"}`, token, "")
			assert.Equal(t, http.StatusBadRequest, rec.Code, target)
		}
	})
	t.Run("needs a login", func(t *testing.T) {
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/staging-auth/session", `{"redirect":"https://shop-staging.webpart.ro/"}`, "", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

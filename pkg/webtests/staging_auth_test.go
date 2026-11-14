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
	req := httptest.NewRequest(http.MethodGet, "/api/v2/staging-auth/verify", nil)
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
	t.Run("basic credentials for automation", func(t *testing.T) {
		good := "Basic " + base64.StdEncoding.EncodeToString([]byte("webpart:"+stagingBasicPassword))
		bad := "Basic " + base64.StdEncoding.EncodeToString([]byte("webpart:nope"))
		assert.Equal(t, http.StatusOK, stagingVerify(e, http.MethodPost, map[string]string{"Authorization": good}, "").Code)
		assert.Equal(t, http.StatusUnauthorized, stagingVerify(e, http.MethodGet, map[string]string{"Authorization": bad}, "").Code)
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

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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/modules/stagingauth"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clientVerify calls the forwardAuth endpoint the way Traefik does (always a GET, the original
// method in X-Forwarded-Method) for a browser request to host+uri.
func clientVerify(e *echo.Echo, verifyPath, host, method, uri, clientCookie string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, verifyPath, nil)
	req.Header.Set("X-Forwarded-Host", host)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Uri", uri)
	req.Header.Set("X-Forwarded-Method", method)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	if clientCookie != "" {
		req.AddCookie(&http.Cookie{Name: stagingauth.ClientCookieName, Value: clientCookie})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func clientCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == stagingauth.ClientCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", stagingauth.ClientCookieName, rec.Header())
	return nil
}

func TestStagingClientLinkAccess(t *testing.T) {
	e := setupStagingAuthTestEnv(t)
	const verify = "/api/v2/staging-auth/verify"
	const shop = "shop-staging.webpart.ro"

	t.Run("a valid key is swapped for a host-only cookie and removed from the URL", func(t *testing.T) {
		rec := clientVerify(e, verify, shop, http.MethodGet, "/ro/produs?access=fixture-active-key&x=1", "")
		require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
		assert.Equal(t, "https://shop-staging.webpart.ro/ro/produs?x=1", rec.Header().Get("Location"))
		c := clientCookieFrom(t, rec)
		assert.Empty(t, c.Domain, "the cookie must be host-only")
		assert.True(t, c.Secure)
		assert.True(t, c.HttpOnly)
		assert.Positive(t, c.MaxAge)

		// The cookie opens this site, for page loads and form posts alike.
		assert.Equal(t, http.StatusOK, clientVerify(e, verify, shop, http.MethodGet, "/", c.Value).Code)
		assert.Equal(t, http.StatusOK, clientVerify(e, verify, shop, http.MethodPost, "/cos", c.Value).Code)
		// ...and no other site.
		assert.Equal(t, http.StatusFound, clientVerify(e, verify, "other-staging.webpart.ro", http.MethodGet, "/", c.Value).Code)
		assert.Equal(t, http.StatusUnauthorized, clientVerify(e, verify, "other-staging.webpart.ro", http.MethodPost, "/", c.Value).Code)
		// Never on the production gate (?basic=0), which only takes a Tasks session.
		assert.Equal(t, http.StatusUnauthorized, clientVerify(e, verify+"?basic=0", shop, http.MethodPost, "/", c.Value).Code)
	})
	t.Run("the key alone keeps a bare path", func(t *testing.T) {
		rec := clientVerify(e, verify, shop, http.MethodGet, "/?access=fixture-active-key", "")
		require.Equal(t, http.StatusFound, rec.Code)
		assert.Equal(t, "https://shop-staging.webpart.ro/", rec.Header().Get("Location"))
	})
	t.Run("expired, unknown or other-site keys get the link-expired page", func(t *testing.T) {
		for _, tc := range []struct{ host, uri string }{
			{shop, "/?access=fixture-expired-key"},
			{shop, "/?access=nope"},
			{"other-staging.webpart.ro", "/?access=fixture-active-key"},
		} {
			rec := clientVerify(e, verify, tc.host, http.MethodGet, tc.uri, "")
			assert.Equal(t, http.StatusForbidden, rec.Code, tc)
			assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
			assert.Contains(t, rec.Body.String(), "Linkul nu mai este valabil")
			assert.Empty(t, rec.Result().Cookies())
		}
	})
	t.Run("keys are ignored on the production gate and on non-page requests", func(t *testing.T) {
		rec := clientVerify(e, verify+"?basic=0", shop, http.MethodGet, "/?access=fixture-active-key", "")
		assert.Equal(t, http.StatusFound, rec.Code)
		assert.True(t, strings.HasPrefix(rec.Header().Get("Location"), "https://tasks.webpart.ro/staging-login"), rec.Header().Get("Location"))
		assert.Equal(t, http.StatusUnauthorized, clientVerify(e, verify, shop, http.MethodPost, "/?access=fixture-active-key", "").Code)
	})
	t.Run("a cookie for an expired link or forged host is refused", func(t *testing.T) {
		rec := clientVerify(e, verify, shop, http.MethodGet, "/?access=fixture-active-key", "")
		c := clientCookieFrom(t, rec)
		assert.Equal(t, http.StatusUnauthorized, clientVerify(e, verify, shop, http.MethodPost, "/", c.Value+"x").Code)
	})
}

func TestStagingClientLinksAPI(t *testing.T) {
	e := setupStagingAuthTestEnv(t)
	token := humaTokenFor(t, &testuser1)
	const base = "/api/v2/staging-auth/client-links"

	t.Run("list needs a login and never returns keys", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, humaRequest(t, e, http.MethodGet, base, "", "", "").Code)
		rec := humaRequest(t, e, http.MethodGet, base, "", token, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "Active client")
		assert.NotContains(t, rec.Body.String(), "token_hash")
	})
	t.Run("create returns a working link for one host", func(t *testing.T) {
		rec := humaRequest(t, e, http.MethodPost, base, `{"label":"Bocavi","host":"https://Bocavi-Staging.webpart.ro/ro/","days":7}`, token, "")
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		var created struct {
			ID   int64  `json:"id"`
			Host string `json:"host"`
			URL  string `json:"url"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
		assert.Equal(t, "bocavi-staging.webpart.ro", created.Host)
		require.True(t, strings.HasPrefix(created.URL, "https://bocavi-staging.webpart.ro/?access="), created.URL)

		uri := strings.TrimPrefix(created.URL, "https://bocavi-staging.webpart.ro")
		v := clientVerify(e, "/api/v2/staging-auth/verify", created.Host, http.MethodGet, uri, "")
		require.Equal(t, http.StatusFound, v.Code, v.Body.String())
		c := clientCookieFrom(t, v)
		assert.Equal(t, http.StatusOK, clientVerify(e, "/api/v2/staging-auth/verify", created.Host, http.MethodPost, "/", c.Value).Code)

		// Revoking it locks the browser out.
		del := humaRequest(t, e, http.MethodDelete, base+"/"+jsonInt(created.ID), "", token, "")
		require.Equal(t, http.StatusNoContent, del.Code, del.Body.String())
		assert.Equal(t, http.StatusUnauthorized, clientVerify(e, "/api/v2/staging-auth/verify", created.Host, http.MethodPost, "/", c.Value).Code)
		assert.Equal(t, http.StatusNotFound, humaRequest(t, e, http.MethodDelete, base+"/"+jsonInt(created.ID), "", token, "").Code)
	})
	t.Run("create refuses hosts outside staging and bad durations", func(t *testing.T) {
		for _, body := range []string{
			`{"label":"x","host":"evil.com"}`,
			`{"label":"x","host":"webpart.ro"}`,
			`{"label":"x","host":"shop-staging.webpart.ro","days":400}`,
			`{"label":"","host":"shop-staging.webpart.ro"}`,
		} {
			rec := humaRequest(t, e, http.MethodPost, base, body, token, "")
			assert.GreaterOrEqual(t, rec.Code, 400, body)
			assert.Less(t, rec.Code, 500, body)
		}
	})
}

func jsonInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

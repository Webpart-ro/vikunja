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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/modules/auth/oauth2server"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	confidentialClientID     = "appwrite"
	confidentialClientSecret = "appwrite-test-secret"
	confidentialRedirectURI  = "https://api.example.com/v1/account/sessions/oauth2/callback/oidc/project"
)

func setupConfidentialClientEnv(t *testing.T) *echo.Echo {
	t.Helper()
	e, err := setupTestEnv()
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(confidentialClientSecret))
	config.ServiceOAuthConfidentialClients.Set([]string{confidentialClientID + ":" + hex.EncodeToString(sum[:])})
	config.ServiceOAuthTrustedRedirectURIs.Set([]string{confidentialRedirectURI})
	t.Cleanup(func() {
		config.ServiceOAuthConfidentialClients.Set([]string{})
		config.ServiceOAuthTrustedRedirectURIs.Set([]string{})
	})
	return e
}

// formTokenRequest posts application/x-www-form-urlencoded to the v2 token endpoint,
// the way Appwrite's OpenID Connect provider does.
func formTokenRequest(e http.Handler, params map[string]string) *httptest.ResponseRecorder {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	e.ServeHTTP(rec, req)
	return rec
}

func confidentialCode(t *testing.T, e http.Handler, clientID string) *httptest.ResponseRecorder {
	t.Helper()
	body := authorizeRequestBody("code", clientID, confidentialRedirectURI, "", "", "st")
	return humaRequest(t, e.(*echo.Echo), http.MethodPost, "/api/v2/oauth/authorize", string(body), humaTokenFor(t, &testuser1), "")
}

func TestOAuth2ConfidentialClient(t *testing.T) {
	t.Run("code without PKCE, secret at the token endpoint, then userinfo", func(t *testing.T) {
		e := setupConfidentialClientEnv(t)
		rec := confidentialCode(t, e, confidentialClientID)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var authz oauth2server.AuthorizeResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &authz))

		rec = formTokenRequest(e, map[string]string{
			"grant_type":    "authorization_code",
			"code":          authz.Code,
			"client_id":     confidentialClientID,
			"client_secret": confidentialClientSecret,
			"redirect_uri":  confidentialRedirectURI,
			"scope":         "openid profile email",
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var tokens oauth2server.TokenResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tokens))
		require.NotEmpty(t, tokens.AccessToken)

		rec = humaRequest(t, e, http.MethodGet, "/api/v2/oauth/userinfo", "", tokens.AccessToken, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var info map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
		assert.Equal(t, "1", info["sub"])
		assert.Equal(t, testuser1.Email, info["email"])
		assert.Equal(t, true, info["email_verified"])
		assert.Equal(t, "user1", info["preferred_username"])
	})

	t.Run("wrong or missing secret is refused and the code is spent", func(t *testing.T) {
		e := setupConfidentialClientEnv(t)
		for _, secret := range []string{"wrong", ""} {
			rec := confidentialCode(t, e, confidentialClientID)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var authz oauth2server.AuthorizeResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &authz))
			params := map[string]string{
				"grant_type":    "authorization_code",
				"code":          authz.Code,
				"client_id":     confidentialClientID,
				"client_secret": secret,
				"redirect_uri":  confidentialRedirectURI,
			}
			rec = formTokenRequest(e, params)
			assert.NotEqual(t, http.StatusOK, rec.Code, "secret %q", secret)
			params["client_secret"] = confidentialClientSecret
			rec = formTokenRequest(e, params)
			assert.NotEqual(t, http.StatusOK, rec.Code, "the code is single-use")
		}
	})

	t.Run("public clients still need PKCE", func(t *testing.T) {
		e := setupConfidentialClientEnv(t)
		rec := confidentialCode(t, e, "docmost")
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	})

	t.Run("userinfo needs a login", func(t *testing.T) {
		e := setupConfidentialClientEnv(t)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/oauth/userinfo", "", "", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

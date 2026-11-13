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
	"net/http"
	"testing"

	"code.vikunja.io/api/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTOTPEnforcementGate(t *testing.T) {
	enforce := func(t *testing.T) {
		config.ServiceEnforceTotp.Set(true)
		t.Cleanup(func() { config.ServiceEnforceTotp.Set(false) })
	}

	t.Run("off by default: account without TOTP can use the API", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/projects", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("enforced: account without TOTP is blocked on v2 and v1", func(t *testing.T) {
		e, err := setupTestEnv() // resets config to defaults, so enforce afterwards
		require.NoError(t, err)
		enforce(t)
		token := humaTokenFor(t, &testuser1)

		for _, path := range []string{"/api/v2/projects", "/api/v1/projects"} {
			rec := humaRequest(t, e, http.MethodGet, path, "", token, "")
			require.Equal(t, http.StatusForbidden, rec.Code, "%s body: %s", path, rec.Body.String())
			assert.Contains(t, rec.Body.String(), `"code":1090`, path)
		}
	})

	t.Run("enforced: enrollment flow stays reachable", func(t *testing.T) {
		e, err := setupTestEnv() // resets config to defaults, so enforce afterwards
		require.NoError(t, err)
		enforce(t)
		token := humaTokenFor(t, &testuser1)

		for _, path := range []string{"/api/v2/user", "/api/v2/user/settings/totp", "/api/v1/user"} {
			rec := humaRequest(t, e, http.MethodGet, path, "", token, "")
			require.Equal(t, http.StatusOK, rec.Code, "%s body: %s", path, rec.Body.String())
		}
	})

	t.Run("enforced: account with TOTP enabled is not blocked", func(t *testing.T) {
		e, err := setupTestEnv() // resets config to defaults, so enforce afterwards
		require.NoError(t, err)
		enforce(t)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/projects", "", humaTokenFor(t, &testuser10), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("enforced: OpenID account is exempt", func(t *testing.T) {
		e, err := setupTestEnv() // resets config to defaults, so enforce afterwards
		require.NoError(t, err)
		enforce(t)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/projects", "", humaTokenFor(t, &testuser14), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("enforced: TOTP cannot be disabled", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		enforce(t)
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/user/settings/totp/disable", `{"password":"12345678","totp_passcode":"000000"}`, humaTokenFor(t, &testuser10), "")
		require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"code":1091`)
	})

	t.Run("enforced: /info reports it, unauthenticated", func(t *testing.T) {
		e, err := setupTestEnv() // resets config to defaults, so enforce afterwards
		require.NoError(t, err)
		enforce(t)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/info", "", "", "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"totp_enforced":true`)
	})
}

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

package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"code.vikunja.io/api/pkg/config"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenIDConfiguration(t *testing.T) {
	config.ServicePublicURL.Set("https://tasks.example.com/")
	e := echo.New()
	e.GET("/.well-known/openid-configuration", OpenIDConfiguration)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
	assert.Equal(t, "https://tasks.example.com", doc["issuer"])
	assert.Equal(t, "https://tasks.example.com/oauth/authorize", doc["authorization_endpoint"])
	assert.Equal(t, "https://tasks.example.com/api/v2/oauth/token", doc["token_endpoint"])
	assert.Equal(t, "https://tasks.example.com/api/v2/oauth/userinfo", doc["userinfo_endpoint"])
}

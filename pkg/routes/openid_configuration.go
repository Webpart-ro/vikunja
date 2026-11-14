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
	"net/http"
	"strings"

	"code.vikunja.io/api/pkg/config"

	"github.com/labstack/echo/v5"
)

// openIDConfiguration is the discovery document of Vikunja's OAuth 2.0 server (Webpart:
// Appwrite's OpenID Connect provider reads the endpoints from it). Vikunja issues no
// id_token, so only the authorization code flow with the userinfo endpoint is advertised.
type openIDConfiguration struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
}

// OpenIDConfiguration serves GET /.well-known/openid-configuration.
func OpenIDConfiguration(c *echo.Context) error {
	base := strings.TrimRight(config.ServicePublicURL.GetString(), "/")
	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	return c.JSON(http.StatusOK, &openIDConfiguration{
		Issuer:                            base,
		AuthorizationEndpoint:             base + "/oauth/authorize",
		TokenEndpoint:                     base + "/api/v2/oauth/token",
		UserinfoEndpoint:                  base + "/api/v2/oauth/userinfo",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		SubjectTypesSupported:             []string{"public"},
		ScopesSupported:                   []string{"openid", "profile", "email"},
		ClaimsSupported:                   []string{"sub", "email", "email_verified", "name", "preferred_username"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "none"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	})
}

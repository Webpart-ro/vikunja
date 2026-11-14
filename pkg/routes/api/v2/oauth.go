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

package apiv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"code.vikunja.io/api/pkg/db"

	"code.vikunja.io/api/pkg/modules/auth/oauth2server"
	"code.vikunja.io/api/pkg/modules/humabridge"
	"code.vikunja.io/api/pkg/user"

	"github.com/danielgtaylor/huma/v2"
)

// oauthTokenBody wraps the OAuth 2.0 token response.
type oauthTokenBody struct {
	// Cache-Control: no-store is required by RFC 6749 §5.1 so tokens are not
	// cached. v2 already sets it globally, but declaring it keeps the contract
	// explicit in the spec.
	CacheControl string `header:"Cache-Control"`
	Body         *oauth2server.TokenResponse
}

// oauthUserinfoBody is the OpenID Connect userinfo response (Webpart: lets Appwrite's
// generic OpenID Connect provider sign people in with their Vikunja account).
type oauthUserinfoBody struct {
	Body struct {
		Sub               string `json:"sub" doc:"The user's id, as a string."`
		Email             string `json:"email" doc:"The user's email address."`
		EmailVerified     bool   `json:"email_verified" doc:"Whether the account is active, i.e. its email address was confirmed."`
		Name              string `json:"name" doc:"The display name, or the username when none is set."`
		PreferredUsername string `json:"preferred_username" doc:"The username."`
	}
}

// oauthAuthorizeBody wraps the OAuth 2.0 authorization response.
type oauthAuthorizeBody struct {
	Body *oauth2server.AuthorizeResponse
}

func init() { AddRouteRegistrar(RegisterOAuthRoutes) }

// RegisterOAuthRoutes wires the OAuth 2.0 token and authorize endpoints. The
// token endpoint is public (it authenticates the request itself); authorize
// inherits the global JWT auth.
func RegisterOAuthRoutes(api huma.API) {
	tags := []string{"auth"}

	Register(api, huma.Operation{
		OperationID:   "oauth-token",
		Summary:       "OAuth 2.0 token endpoint",
		Description:   "Exchanges an authorization code (grant_type=authorization_code) or a refresh token (grant_type=refresh_token) for an access token. Accepts application/x-www-form-urlencoded per RFC 6749 as well as JSON.",
		Method:        http.MethodPost,
		Path:          "/oauth/token",
		DefaultStatus: http.StatusOK,
		Tags:          tags,
		Security:      publicSecurity,
		Middlewares:   huma.Middlewares{formBodyAsJSON(api)},
	}, oauthToken)

	Register(api, huma.Operation{
		OperationID:   "oauth-authorize",
		Summary:       "OAuth 2.0 authorize endpoint",
		Description:   "Creates a single-use authorization code for the authenticated user. PKCE (code_challenge with method S256) and a loopback or vikunja- scheme redirect_uri are required.",
		Method:        http.MethodPost,
		Path:          "/oauth/authorize",
		DefaultStatus: http.StatusOK,
		Tags:          tags,
	}, oauthAuthorize)

	Register(api, huma.Operation{
		OperationID: "oauth-userinfo",
		Summary:     "OpenID Connect userinfo",
		Description: "Returns the signed-in user's identity in OpenID Connect userinfo form (sub, email, email_verified, name, preferred_username), for apps that log people in through Vikunja's OAuth 2.0 server. Needs a user access token; API tokens and link shares are refused with 403.",
		Method:      http.MethodGet,
		Path:        "/oauth/userinfo",
		Tags:        tags,
	}, oauthUserinfo)
}

// Per operation: an API-wide form format would let a cross-site <form> drive every endpoint (GHSA-frf4-3prf-3vch).
func formBodyAsJSON(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		mediaType, _, _ := mime.ParseMediaType(ctx.Header("Content-Type"))
		if mediaType != "application/x-www-form-urlencoded" {
			next(ctx)
			return
		}
		body, err := formToJSON(ctx.BodyReader())
		if errors.Is(err, errFormBodyTooLarge) {
			_ = huma.WriteErr(api, ctx, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusBadRequest, "invalid form body", err)
			return
		}
		next(&jsonBodyContext{humaContext: ctx, body: bytes.NewReader(body)})
	}
}

// Read before Huma's MaxBodyBytes applies, so it needs its own cap.
const maxFormBodyBytes = 64 << 10

var errFormBodyTooLarge = errors.New("form body too large")

// Only string scalars are produced, which is all TokenRequest needs.
func formToJSON(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxFormBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxFormBodyBytes {
		return nil, errFormBodyTooLarge
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, err
	}
	flat := make(map[string]string, len(values))
	for key := range values {
		flat[key] = values.Get(key)
	}
	return json.Marshal(flat)
}

// Alias so the embedded field isn't named Context, which would shadow huma.Context.Context().
type humaContext = huma.Context

type jsonBodyContext struct {
	humaContext
	body io.Reader
}

func (c *jsonBodyContext) BodyReader() io.Reader { return c.body }

func (c *jsonBodyContext) Header(name string) string {
	if http.CanonicalHeaderKey(name) == "Content-Type" {
		return "application/json"
	}
	return c.humaContext.Header(name)
}

func oauthToken(ctx context.Context, in *struct {
	Body oauth2server.TokenRequest `contentType:"application/x-www-form-urlencoded"`
}) (*oauthTokenBody, error) {
	deviceInfo, ipAddress := requestClientInfo(ctx)
	resp, err := oauth2server.ExchangeToken(ctx, &in.Body, deviceInfo, ipAddress)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &oauthTokenBody{CacheControl: "no-store", Body: resp}, nil
}

func oauthAuthorize(ctx context.Context, in *struct{ Body oauth2server.AuthorizeRequest }) (*oauthAuthorizeBody, error) {
	if ec := humabridge.EchoContextFrom(ctx); ec != nil && (*ec).Get("api_token") != nil {
		return nil, huma.Error403Forbidden("API tokens cannot be used to authorize OAuth clients")
	}

	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	u, err := user.GetFromAuth(a)
	if err != nil {
		return nil, translateDomainError(err)
	}

	resp, err := oauth2server.Authorize(&in.Body, u.ID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &oauthAuthorizeBody{Body: resp}, nil
}

func oauthUserinfo(ctx context.Context, _ *struct{}) (*oauthUserinfoBody, error) {
	if ec := humabridge.EchoContextFrom(ctx); ec != nil && (*ec).Get("api_token") != nil {
		return nil, huma.Error403Forbidden("API tokens cannot be used for userinfo")
	}
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if _, ok := a.(*user.User); !ok {
		return nil, huma.Error403Forbidden("userinfo needs a user login")
	}
	s := db.NewSession()
	defer s.Close()
	u, err := user.GetUserWithEmail(s, &user.User{ID: a.GetID()})
	if err != nil {
		return nil, translateDomainError(err)
	}
	out := &oauthUserinfoBody{}
	out.Body.Sub = strconv.FormatInt(u.ID, 10)
	out.Body.Email = u.Email
	out.Body.EmailVerified = u.Status == user.StatusActive && u.Email != ""
	out.Body.Name = u.Name
	if out.Body.Name == "" {
		out.Body.Name = u.Username
	}
	out.Body.PreferredUsername = u.Username
	return out, nil
}

// requestClientInfo pulls the user agent and client IP off the underlying Echo
// request so the authorization_code grant (and login) can record them on the
// session they create, mirroring v1. Both fall back to "" when the context is
// unavailable.
func requestClientInfo(ctx context.Context) (deviceInfo, ipAddress string) {
	ec := humabridge.EchoContextFrom(ctx)
	if ec == nil {
		return "", ""
	}
	return (*ec).Request().UserAgent(), (*ec).RealIP()
}

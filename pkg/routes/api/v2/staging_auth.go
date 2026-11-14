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

// Webpart staging single sign-on: Traefik's forwardAuth middleware asks /staging-auth/verify
// whether a request to a staging website may pass; browsers without a session are sent to
// the /staging-login page, which calls /staging-auth/session to get the shared cookie.

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/modules/humabridge"
	"code.vikunja.io/api/pkg/modules/stagingauth"
	"code.vikunja.io/api/pkg/user"

	"github.com/danielgtaylor/huma/v2"
)

type stagingVerifyInput struct {
	Host          string `header:"X-Forwarded-Host" doc:"Set by the proxy: the host of the original request."`
	Proto         string `header:"X-Forwarded-Proto" doc:"Set by the proxy: the scheme of the original request."`
	URI           string `header:"X-Forwarded-Uri" doc:"Set by the proxy: path and query of the original request."`
	Method        string `header:"X-Forwarded-Method" doc:"Set by the proxy: the method of the original request."`
	Accept        string `header:"Accept" doc:"The original request's Accept header; only page loads (text/html) are redirected to the login page."`
	Authorization string `header:"Authorization" doc:"Optional HTTP Basic credentials for automation."`
	Session       string `cookie:"webpart_staging" doc:"The staging session cookie."`
}

type stagingVerifyOutput struct {
	Status   int
	Location string `header:"Location" doc:"Where an unauthenticated browser is sent to log in."`
	Cache    string `header:"Cache-Control"`
}

type stagingSessionInput struct {
	Body struct {
		Redirect string `json:"redirect" doc:"The staging URL to return to; must be https on the configured cookie domain."`
	}
}

type stagingSessionOutput struct {
	Body struct {
		Redirect  string    `json:"redirect" doc:"The validated URL to send the browser to."`
		ExpiresAt time.Time `json:"expires_at" doc:"When the staging session ends."`
	}
}

// RegisterStagingAuthRoutes wires staging SSO onto the Huma API, gated by stagingauth.enabled.
func RegisterStagingAuthRoutes(api huma.API) {
	if !stagingauth.Enabled() {
		return
	}
	tags := []string{"staging-auth"}

	Register(api, huma.Operation{
		OperationID: "staging-auth-verify",
		Summary:     "Check a staging website request (forwardAuth)",
		Description: "Called by the reverse proxy for every request to a protected staging website, with the original request's X-Forwarded-* headers and cookies. 200 lets it through: a valid staging session cookie or HTTP Basic credentials listed in stagingauth.basicauthusers. A browser page load (GET/HEAD accepting text/html) without a session gets a 302 to the login page, which returns to the original URL; anything else gets a 401.",
		Method:      http.MethodGet, Path: "/staging-auth/verify", Tags: tags,
		Security: []map[string][]string{},
	}, stagingAuthVerify)

	Register(api, huma.Operation{
		OperationID: "staging-auth-session",
		Summary:     "Start a staging session",
		Description: "Sets the staging session cookie on the configured cookie domain for the logged-in user and returns the validated redirect target. Only for interactive user sessions: API tokens, link shares and bots are refused with 403. Mandatory 2FA applies like on any other request.",
		Method:      http.MethodPost, Path: "/staging-auth/session", Tags: tags,
		DefaultStatus: http.StatusOK,
	}, stagingAuthSession)
}

func init() { AddRouteRegistrar(RegisterStagingAuthRoutes) }

func stagingAuthVerify(_ context.Context, in *stagingVerifyInput) (*stagingVerifyOutput, error) {
	out := &stagingVerifyOutput{Status: http.StatusOK, Cache: "no-store"}
	now := time.Now()

	if in.Session != "" {
		userID, err := stagingauth.ParseSessionToken(in.Session)
		if err == nil && stagingUserActive(userID, now) {
			return out, nil
		}
	}
	if strings.HasPrefix(in.Authorization, "Basic ") {
		if stagingauth.ValidBasicAuth(in.Authorization, now) {
			return out, nil
		}
		out.Status = http.StatusUnauthorized
		return out, nil
	}

	// Only browser page loads go to the login page; scripts, assets and health checks
	// (curl expects the 401 the password wall used to give) are refused.
	method := strings.ToUpper(in.Method)
	host := strings.TrimSpace(strings.Split(in.Host, ",")[0])
	pageLoad := method == "" || method == http.MethodGet || method == http.MethodHead
	if pageLoad && strings.Contains(in.Accept, "text/html") && stagingauth.HostAllowed(stripPort(host)) {
		uri := in.URI
		if !strings.HasPrefix(uri, "/") {
			uri = "/" + uri
		}
		out.Status = http.StatusFound
		out.Location = stagingauth.LoginURL("https://" + host + uri)
		return out, nil
	}
	out.Status = http.StatusUnauthorized
	return out, nil
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[i:], "]") {
		return host[:i]
	}
	return host
}

// stagingUserCache keeps the "is this user still active" answer for a minute: forwardAuth runs
// on every staging request (pages and assets), a disabled user is locked out within a minute.
var stagingUserCache = struct {
	sync.Mutex
	until map[int64]time.Time
}{until: map[int64]time.Time{}}

func stagingUserActive(userID int64, now time.Time) bool {
	stagingUserCache.Lock()
	if until, ok := stagingUserCache.until[userID]; ok && now.Before(until) {
		stagingUserCache.Unlock()
		return true
	}
	stagingUserCache.Unlock()

	s := db.NewSession()
	defer s.Close()
	u, err := user.GetUserByID(s, userID)
	if err != nil {
		if !user.IsErrUserDoesNotExist(err) {
			log.Errorf("staging auth: could not load user %d: %s", userID, err)
		}
		return false
	}
	if u.Status != user.StatusActive || u.IsBot() {
		return false
	}
	stagingUserCache.Lock()
	stagingUserCache.until[userID] = now.Add(time.Minute)
	stagingUserCache.Unlock()
	return true
}

func stagingAuthSession(ctx context.Context, in *stagingSessionInput) (*stagingSessionOutput, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	u, ok := a.(*user.User)
	ec := humabridge.EchoContextFrom(ctx)
	if !ok || ec == nil || ec.Get("api_token") != nil {
		return nil, huma.Error403Forbidden("staging sessions need an interactive login")
	}

	target, err := stagingauth.ValidateRedirect(in.Body.Redirect)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	s := db.NewSession()
	fresh, err := user.GetUserByID(s, u.ID)
	_ = s.Close()
	if err != nil {
		return nil, translateDomainError(err)
	}
	if fresh.IsBot() || fresh.Status != user.StatusActive {
		return nil, huma.Error403Forbidden("staging sessions need an interactive login")
	}

	now := time.Now()
	token, err := stagingauth.NewSessionToken(fresh.ID, now)
	if err != nil {
		return nil, err
	}
	duration := stagingauth.SessionDuration()
	ec.SetCookie(&http.Cookie{ //nolint:gosec // G124: Secure follows service.publicurl, like the refresh cookie.
		Name:     stagingauth.CookieName,
		Value:    token,
		Domain:   stagingauth.CookieDomain(),
		Path:     "/",
		MaxAge:   int(duration.Seconds()),
		HttpOnly: true,
		Secure:   strings.HasPrefix(config.ServicePublicURL.GetString(), "https"),
		SameSite: http.SameSiteLaxMode,
	})

	out := &stagingSessionOutput{}
	out.Body.Redirect = target.String()
	out.Body.ExpiresAt = now.Add(duration)
	return out, nil
}

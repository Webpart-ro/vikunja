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
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/log"
	auth2 "code.vikunja.io/api/pkg/modules/auth"
	"code.vikunja.io/api/pkg/user"

	"github.com/labstack/echo/v5"
)

// totpEnrollmentPaths are the only authenticated requests a local account without
// TOTP may make while service.enforcetotp is on: enough to load the own profile and
// settings, set up TOTP, keep the session alive and log out.
var totpEnrollmentPaths = map[string]bool{
	"/api/v1/user":                      true,
	"/api/v1/user/token":                true,
	"/api/v1/user/logout":               true,
	"/api/v1/user/timezones":            true,
	"/api/v1/user/settings/general":     true,
	"/api/v1/user/settings/totp":        true,
	"/api/v1/user/settings/totp/enroll": true,
	"/api/v1/user/settings/totp/enable": true,
	"/api/v1/user/settings/totp/qrcode": true,
	"/api/v1/token/test":                true,

	"/api/v2/user":                      true,
	"/api/v2/user/token":                true,
	"/api/v2/user/token/refresh":        true,
	"/api/v2/logout":                    true,
	"/api/v2/user/timezones":            true,
	"/api/v2/user/settings/general":     true,
	"/api/v2/user/settings/totp":        true,
	"/api/v2/user/settings/totp/enroll": true,
	"/api/v2/user/settings/totp/enable": true,
	"/api/v2/user/settings/totp/qrcode": true,
}

func isTOTPEnrollmentPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	if totpEnrollmentPaths[path] {
		return true
	}
	// Avatars render in the header while the user sets up TOTP.
	return strings.HasPrefix(path, "/api/v1/avatar/") || strings.HasPrefix(path, "/api/v2/avatar/")
}

func isTOTPDisablePath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	return path == "/api/v1/user/settings/totp/disable" || path == "/api/v2/user/settings/totp/disable"
}

// RequireTOTPEnrollment blocks local accounts that have not enabled TOTP from every
// authenticated request except the enrollment flow, when service.enforcetotp is on.
// Link shares, bots and accounts from external (OpenID) issuers are not affected.
// Must run after SetupTokenMiddleware so the request's auth is in the context.
func RequireTOTPEnrollment() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !config.ServiceEnableTotp.GetBool() || !config.ServiceEnforceTotp.GetBool() {
				return next(c)
			}
			// While 2FA is mandatory nobody may switch it off again.
			if isTOTPDisablePath(c.Request().URL.Path) {
				return c.JSON(http.StatusForbidden, (&user.ErrTOTPCannotBeDisabled{}).HTTPError())
			}
			if isTOTPEnrollmentPath(c.Request().URL.Path) {
				return next(c)
			}

			a, err := auth2.GetAuthFromClaims(c)
			if err != nil {
				// Unauthenticated routes (login, info, …) carry no claims; the token
				// middleware already rejected requests that needed them.
				return next(c)
			}
			u, ok := a.(*user.User)
			if !ok {
				return next(c) // link share
			}

			// Close before next(): SQLite deadlocks when a read session is held across a write.
			s := db.NewSession()
			fresh, err := user.GetUserByID(s, u.ID)
			if err != nil {
				_ = s.Close()
				log.Errorf("TOTP gate: could not load user %d: %s", u.ID, err)
				return echo.ErrInternalServerError
			}
			if fresh.Issuer != user.IssuerLocal || fresh.IsBot() || isExemptAPITokenRequest(c, fresh.Username) {
				_ = s.Close()
				return next(c)
			}
			enabled, err := user.TOTPEnabledForUser(s, fresh)
			_ = s.Close()
			if err != nil {
				log.Errorf("TOTP gate: could not check TOTP for user %d: %s", u.ID, err)
				return echo.ErrInternalServerError
			}
			if !enabled {
				return c.JSON(http.StatusForbidden, (&user.ErrTOTPEnrollmentRequired{UserID: u.ID}).HTTPError())
			}
			return next(c)
		}
	}
}

// isExemptAPITokenRequest: service accounts listed in service.enforcetotpexemptusers keep
// API-token access (CI automation can't type TOTP codes). Their browser sessions are not exempt.
func isExemptAPITokenRequest(c *echo.Context, username string) bool {
	if c.Get("api_token") == nil {
		return false
	}
	for _, entry := range config.ServiceEnforceTotpExemptUsers.GetStringSlice() {
		for _, name := range strings.Split(entry, ",") {
			if name = strings.TrimSpace(name); name != "" && strings.EqualFold(name, username) {
				return true
			}
		}
	}
	return false
}

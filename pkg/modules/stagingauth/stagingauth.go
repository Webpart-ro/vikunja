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

// Package stagingauth is Webpart's single sign-on for staging websites: a reverse proxy
// (Traefik forwardAuth) asks Vikunja whether a request may pass. Browsers pass with a
// signed cookie shared across the cookie domain (set after a normal Vikunja login, so
// mandatory 2FA applies); automation passes with HTTP Basic credentials (htpasswd lines).
package stagingauth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/config"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// CookieName is the cookie the staging session travels in.
const CookieName = "webpart_staging"

const tokenType = "staging"

// Enabled reports whether staging SSO is configured.
func Enabled() bool {
	return config.StagingAuthEnabled.GetBool() && CookieDomain() != ""
}

// CookieDomain is the registrable domain the session cookie is shared on (e.g. webpart.ro).
func CookieDomain() string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(config.StagingAuthCookieDomain.GetString())), ".")
}

// SessionDuration is how long a staging session stays valid.
func SessionDuration() time.Duration {
	days := config.StagingAuthSessionDays.GetInt()
	if days <= 0 {
		days = 14
	}
	return time.Duration(days) * 24 * time.Hour
}

// HostAllowed reports whether host (without port) is the cookie domain or one of its subdomains.
func HostAllowed(host string) bool {
	domain := CookieDomain()
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if domain == "" || host == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// ErrInvalidRedirect is returned for redirect targets outside the cookie domain.
var ErrInvalidRedirect = errors.New("redirect target must be an https URL on the staging domain")

// ValidateRedirect parses a post-login redirect target and only accepts absolute https URLs
// on the cookie domain, so the login page can't be abused as an open redirect.
func ValidateRedirect(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || !HostAllowed(u.Hostname()) {
		return nil, ErrInvalidRedirect
	}
	return u, nil
}

// NewSessionToken signs a staging session for a user.
func NewSessionToken(userID int64, now time.Time) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ": tokenType,
		"sub": fmt.Sprintf("%d", userID),
		"iat": now.Unix(),
		"exp": now.Add(SessionDuration()).Unix(),
	})
	return t.SignedString([]byte(config.ServiceSecret.GetString()))
}

// ParseSessionToken verifies a staging session cookie and returns its user ID.
func ParseSessionToken(token string) (int64, error) {
	parsed, err := jwt.Parse(token, func(_ *jwt.Token) (any, error) {
		return []byte(config.ServiceSecret.GetString()), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return 0, err
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || claims["typ"] != tokenType {
		return 0, errors.New("not a staging session token")
	}
	sub, _ := claims["sub"].(string)
	var id int64
	if _, err := fmt.Sscanf(sub, "%d", &id); err != nil || id <= 0 {
		return 0, errors.New("staging session token has no user")
	}
	return id, nil
}

// basicCache remembers verified Basic credentials briefly: bcrypt is deliberately slow and
// forwardAuth runs on every request (pages, assets, health checks).
var basicCache = struct {
	sync.Mutex
	until map[string]time.Time
}{until: map[string]time.Time{}}

const basicCacheTTL = 5 * time.Minute

// ValidBasicAuth checks an "Authorization: Basic …" header against stagingauth.basicauthusers
// (htpasswd "user:bcrypt-hash" lines).
func ValidBasicAuth(header string, now time.Time) bool {
	encoded, ok := strings.CutPrefix(header, "Basic ")
	if !ok {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return false
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok || username == "" {
		return false
	}

	cacheKey := string(decoded)
	basicCache.Lock()
	if until, hit := basicCache.until[cacheKey]; hit && now.Before(until) {
		basicCache.Unlock()
		return true
	}
	basicCache.Unlock()

	for _, line := range basicUsers() {
		name, hash, found := strings.Cut(line, ":")
		if !found || name != username {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
			basicCache.Lock()
			if len(basicCache.until) > 100 {
				basicCache.until = map[string]time.Time{}
			}
			basicCache.until[cacheKey] = now.Add(basicCacheTTL)
			basicCache.Unlock()
			return true
		}
	}
	return false
}

func basicUsers() []string {
	lines := []string{}
	for _, entry := range config.StagingAuthBasicAuthUsers.GetStringSlice() {
		for _, line := range strings.Split(entry, ",") {
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, line)
			}
		}
	}
	return lines
}

// LoginURL is the Vikunja page that signs a browser in and sends it back to target.
func LoginURL(target string) string {
	base := strings.TrimRight(config.ServicePublicURL.GetString(), "/")
	return base + "/staging-login?rd=" + url.QueryEscape(target)
}

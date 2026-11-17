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
package stagingauth

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/config"

	"github.com/golang-jwt/jwt/v5"
)

// Client links: a private URL (https://<host>/?access=<key>) that opens one staging website
// for someone without a Tasks account. The verify endpoint swaps the key for ClientCookieName,
// a host-only cookie, so the browser never sees the key again and it never reaches the site.

// ClientCookieName is the host-only cookie a client link session travels in.
const ClientCookieName = "webpart_client"

// ClientKeyParam is the query parameter that carries a client link key.
const ClientKeyParam = "access"

const clientTokenType = "staging-client"

// ErrInvalidClientHost is returned for a host a client link can't be made for.
var ErrInvalidClientHost = errors.New("host must be a subdomain of the staging domain, e.g. shop-staging.webpart.ro")

// NormalizeClientHost accepts a bare host or a URL and returns the lowercase host, which must be
// a subdomain of the cookie domain (never the cookie domain itself).
func NormalizeClientHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", ErrInvalidClientHost
		}
		raw = u.Hostname()
	}
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(raw, "/"), "."))
	if host == "" || strings.ContainsAny(host, "/:@?# ") || host == CookieDomain() || !HostAllowed(host) {
		return "", ErrInvalidClientHost
	}
	return host, nil
}

// ClientLinkURL is the URL handed to the client.
func ClientLinkURL(host, key string) string {
	return "https://" + host + "/?" + ClientKeyParam + "=" + url.QueryEscape(key)
}

// NewClientToken signs a client session for one link and host, valid until the link expires.
func NewClientToken(linkID int64, host string, expiresAt, now time.Time) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ":  clientTokenType,
		"sub":  fmt.Sprintf("%d", linkID),
		"host": host,
		"iat":  now.Unix(),
		"exp":  expiresAt.Unix(),
	})
	return t.SignedString([]byte(config.ServiceSecret.GetString()))
}

// ParseClientToken verifies a client session cookie and returns its link id and host.
func ParseClientToken(token string) (int64, string, error) {
	parsed, err := jwt.Parse(token, func(_ *jwt.Token) (any, error) {
		return []byte(config.ServiceSecret.GetString()), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return 0, "", err
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || claims["typ"] != clientTokenType {
		return 0, "", errors.New("not a staging client token")
	}
	host, _ := claims["host"].(string)
	sub, _ := claims["sub"].(string)
	var id int64
	if _, err := fmt.Sscanf(sub, "%d", &id); err != nil || id <= 0 || host == "" {
		return 0, "", errors.New("staging client token has no link")
	}
	return id, host, nil
}

// SplitClientKey removes the access parameter from a request URI ("/path?a=1&access=k") and
// returns the key and the URI without it. The key is empty when the parameter is absent.
func SplitClientKey(uri string) (key, rest string) {
	path, rawQuery, hasQuery := strings.Cut(uri, "?")
	if !hasQuery {
		return "", uri
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil || !values.Has(ClientKeyParam) {
		return "", uri
	}
	key = values.Get(ClientKeyParam)
	values.Del(ClientKeyParam)
	if encoded := values.Encode(); encoded != "" {
		return key, path + "?" + encoded
	}
	return key, path
}

// ClientLinkInvalidHTML is shown when a client link has expired, was revoked, or belongs to
// another site. Team members can still log in with Tasks from here.
func ClientLinkInvalidHTML(target string) string {
	login := html.EscapeString(LoginURL(target))
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"ro\"><head><meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<meta name=\"robots\" content=\"noindex, nofollow\">\n")
	b.WriteString("<title>Webpart · link expirat</title>\n")
	b.WriteString("<style>body{font-family:system-ui,sans-serif;max-width:32rem;margin:15vh auto;padding:0 1rem;color:#10312b;line-height:1.5}a{color:#10312b}small{color:#5b6b67}</style>\n")
	b.WriteString("</head><body>\n")
	b.WriteString("<h1>Linkul nu mai este valabil</h1>\n")
	b.WriteString("<p>Linkul de acces pentru această pagină a expirat sau a fost dezactivat. Cere un link nou de la Webpart.</p>\n")
	b.WriteString("<p><small>This access link has expired or was revoked. Ask Webpart for a new one.</small></p>\n")
	b.WriteString("<p><small><a href=\"" + login + "\">Echipa Webpart: autentificare cu Tasks</a></small></p>\n")
	b.WriteString("</body></html>\n")
	return b.String()
}

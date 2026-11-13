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

package oauth2server

import (
	"net"
	"net/url"
	"strings"

	"code.vikunja.io/api/pkg/config"
)

// ValidateRedirectURI checks that the redirect_uri is either a Vikunja native
// app scheme (e.g. vikunja-flutter://callback) or a loopback http URL as
// recommended by RFC 8252 for native apps that cannot register a custom
// scheme. Any address in 127.0.0.0/8, the IPv6 loopback (::1, in any
// notation), and the literal hostname "localhost" are accepted; dangerous
// schemes like javascript:, data:, or non-loopback http:// targets are
// rejected. https:// targets are only accepted when they exactly match an
// entry in service.oauthtrustedredirecturis (Webpart: wiki login via Vikunja).
func ValidateRedirectURI(redirectURI string) bool {
	u, err := url.Parse(redirectURI)
	if err != nil || u.Scheme == "" {
		return false
	}

	if strings.HasPrefix(u.Scheme, "vikunja-") {
		return true
	}

	if u.Scheme == "https" && isTrustedRedirectURI(redirectURI) {
		return true
	}

	if u.Scheme == "http" {
		host := u.Hostname()
		if strings.EqualFold(host, "localhost") {
			return true
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return true
		}
	}

	return false
}

// isTrustedRedirectURI reports whether redirectURI exactly matches one of the
// operator-configured service.oauthtrustedredirecturis entries. Exact match only:
// no prefixes or wildcards, so an attacker can't append a path or query.
func isTrustedRedirectURI(redirectURI string) bool {
	for _, entry := range config.ServiceOAuthTrustedRedirectURIs.GetStringSlice() {
		// An env var arrives as one comma-separated string.
		for _, trusted := range strings.Split(entry, ",") {
			trusted = strings.TrimSpace(trusted)
			if trusted != "" && trusted == redirectURI {
				return true
			}
		}
	}
	return false
}

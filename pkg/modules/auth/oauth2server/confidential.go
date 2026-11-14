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
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"code.vikunja.io/api/pkg/config"
)

// Webpart: confidential clients. Server-side apps that can keep a secret but can't do PKCE
// (Appwrite's generic OpenID Connect provider) authenticate the code exchange with a
// client_secret instead. Configured as "client_id:sha256-hex-of-secret" entries in
// service.oauthconfidentialclients, so the secret itself never sits in Vikunja's config.

func confidentialClientHash(clientID string) (string, bool) {
	if clientID == "" {
		return "", false
	}
	for _, entry := range config.ServiceOAuthConfidentialClients.GetStringSlice() {
		for _, item := range strings.Split(entry, ",") {
			id, hash, ok := strings.Cut(strings.TrimSpace(item), ":")
			if ok && id == clientID && hash != "" {
				return strings.ToLower(hash), true
			}
		}
	}
	return "", false
}

// IsConfidentialClient reports whether clientID is a registered confidential client.
func IsConfidentialClient(clientID string) bool {
	_, ok := confidentialClientHash(clientID)
	return ok
}

// ValidClientSecret checks a confidential client's secret in constant time.
func ValidClientSecret(clientID, secret string) bool {
	want, ok := confidentialClientHash(clientID)
	if !ok || secret == "" {
		return false
	}
	sum := sha256.Sum256([]byte(secret))
	got := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

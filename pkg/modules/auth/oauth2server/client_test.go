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
	"testing"

	"code.vikunja.io/api/pkg/config"

	"github.com/stretchr/testify/assert"
)

func TestValidateRedirectURI(t *testing.T) {
	t.Run("accepts vikunja-flutter scheme", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("vikunja-flutter://callback"))
	})
	t.Run("accepts vikunja-desktop scheme", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("vikunja-desktop://auth"))
	})
	t.Run("accepts http localhost", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("http://localhost/callback"))
	})
	t.Run("accepts http localhost with port", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("http://localhost:8080/callback"))
	})
	t.Run("accepts http 127.0.0.1", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("http://127.0.0.1:8080/callback"))
	})
	t.Run("accepts other 127.0.0.0/8 loopback addresses", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("http://127.0.0.2:1234/callback"))
	})
	t.Run("accepts http ipv6 loopback", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("http://[::1]:8080/callback"))
	})
	t.Run("accepts expanded ipv6 loopback", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("http://[0:0:0:0:0:0:0:1]:1234/callback"))
	})
	t.Run("accepts localhost case-insensitively", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("http://LocalHost:8080/callback"))
	})
	t.Run("rejects 0.0.0.0", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("http://0.0.0.0:8080/callback"))
	})
	t.Run("rejects https scheme", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("https://evil.com/callback"))
	})
	t.Run("rejects https localhost", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("https://localhost/callback"))
	})
	t.Run("rejects non-loopback http", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("http://evil.com/callback"))
	})
	t.Run("rejects http with localhost in path", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("http://evil.com/localhost"))
	})
	t.Run("rejects http with localhost subdomain trick", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("http://localhost.evil.com/callback"))
	})
	t.Run("rejects javascript scheme", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("javascript:alert(1)"))
	})
	t.Run("rejects data scheme", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("data:text/html,<script>alert(1)</script>"))
	})
	t.Run("rejects non-vikunja custom scheme", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("myapp://callback"))
	})
	t.Run("rejects empty URI", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI(""))
	})
}

func TestValidateRedirectURITrustedHTTPS(t *testing.T) {
	config.ServiceOAuthTrustedRedirectURIs.Set([]string{"https://wiki.example.com/api/auth/vikunja/callback, https://other.example.com/cb"})
	t.Cleanup(func() { config.ServiceOAuthTrustedRedirectURIs.Set([]string{}) })

	t.Run("accepts an exactly configured https uri", func(t *testing.T) {
		assert.True(t, ValidateRedirectURI("https://wiki.example.com/api/auth/vikunja/callback"))
		assert.True(t, ValidateRedirectURI("https://other.example.com/cb"))
	})
	t.Run("rejects a different path on a trusted host", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("https://wiki.example.com/evil"))
	})
	t.Run("rejects an appended query", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("https://wiki.example.com/api/auth/vikunja/callback?x=1"))
	})
	t.Run("rejects an untrusted https host", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("https://attacker.example.com/api/auth/vikunja/callback"))
	})
	t.Run("rejects http even if the rest matches", func(t *testing.T) {
		assert.False(t, ValidateRedirectURI("http://wiki.example.com/api/auth/vikunja/callback"))
	})
}

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
	"encoding/base64"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func setup(t *testing.T) {
	t.Helper()
	config.ServiceSecret.Set("test-secret")
	config.StagingAuthEnabled.Set(true)
	config.StagingAuthCookieDomain.Set(".Webpart.ro")
	config.StagingAuthSessionDays.Set(14)
	config.ServicePublicURL.Set("https://tasks.webpart.ro/")
}

func TestHostAllowed(t *testing.T) {
	setup(t)
	assert.True(t, Enabled())
	assert.True(t, HostAllowed("webpart.ro"))
	assert.True(t, HostAllowed("webpart-ro-staging.webpart.ro"))
	assert.True(t, HostAllowed("A.B.WEBPART.RO."))
	assert.False(t, HostAllowed("evilwebpart.ro"))
	assert.False(t, HostAllowed("webpart.ro.evil.com"))
	assert.False(t, HostAllowed(""))
}

func TestValidateRedirect(t *testing.T) {
	setup(t)
	u, err := ValidateRedirect("https://webpart-ro-staging.webpart.ro/ro/contact?x=1")
	require.NoError(t, err)
	assert.Equal(t, "/ro/contact", u.Path)

	for _, bad := range []string{
		"http://webpart-ro-staging.webpart.ro/",
		"https://evil.com/",
		"https://evilwebpart.ro/",
		"https://user:pw@x.webpart.ro/",
		"//x.webpart.ro/",
		"javascript:alert(1)",
		"",
	} {
		_, err := ValidateRedirect(bad)
		assert.ErrorIs(t, err, ErrInvalidRedirect, bad)
	}
}

func TestSessionToken(t *testing.T) {
	setup(t)
	now := time.Now()
	token, err := NewSessionToken(42, now)
	require.NoError(t, err)

	id, err := ParseSessionToken(token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), id)

	expired, err := NewSessionToken(42, now.Add(-15*24*time.Hour))
	require.NoError(t, err)
	_, err = ParseSessionToken(expired)
	require.Error(t, err)

	_, err = ParseSessionToken(token + "x")
	require.Error(t, err)

	config.ServiceSecret.Set("other-secret")
	_, err = ParseSessionToken(token)
	require.Error(t, err)
}

func TestValidBasicAuth(t *testing.T) {
	setup(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	require.NoError(t, err)
	// Traefik/htpasswd write $2y$ hashes.
	config.StagingAuthBasicAuthUsers.Set([]string{"webpart:" + "$2y$" + string(hash[4:])})

	header := func(user, pass string) string {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	}
	now := time.Now()
	assert.True(t, ValidBasicAuth(header("webpart", "s3cret"), now))
	assert.True(t, ValidBasicAuth(header("webpart", "s3cret"), now)) // cached
	assert.False(t, ValidBasicAuth(header("webpart", "wrong"), now))
	assert.False(t, ValidBasicAuth(header("other", "s3cret"), now))
	assert.False(t, ValidBasicAuth("Bearer abc", now))
	assert.False(t, ValidBasicAuth("Basic !!!", now))
}

func TestLoginURL(t *testing.T) {
	setup(t)
	assert.Equal(t,
		"https://tasks.webpart.ro/staging-login?rd=https%3A%2F%2Fa.webpart.ro%2Fx%3Fy%3D1",
		LoginURL("https://a.webpart.ro/x?y=1"))
}

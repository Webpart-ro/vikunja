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

// Package github is the Webpart GitHub integration: a small REST client that
// authenticates as a GitHub App and reads/creates the branches, pull requests,
// CI runs and commits tasks are linked to by their identifier (e.g. HS-334).
package github

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/utils"

	"github.com/golang-jwt/jwt/v5"
)

// ErrNotConfigured is returned when the integration is off or incomplete.
var ErrNotConfigured = errors.New("github integration is not configured")

// ErrRepoNotAccessible is returned when the GitHub App is not installed on a repository.
var ErrRepoNotAccessible = errors.New("the GitHub App is not installed on this repository")

// APIError is a non-2xx response from GitHub.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github api: %d %s", e.Status, e.Message)
}

// Client talks to the GitHub REST API as a GitHub App installation.
type Client struct {
	apiURL string
	appID  string
	key    *rsa.PrivateKey
	http   *http.Client
	now    func() time.Time

	mu            sync.Mutex
	installations map[string]int64 // "owner/repo" -> installation id
	tokens        map[int64]cachedToken
	devCache      map[string]cachedDevelopment
}

type cachedToken struct {
	token   string
	expires time.Time
}

// NewClient builds a client from explicit settings (used by tests and Default).
func NewClient(apiURL, appID string, key *rsa.PrivateKey, httpClient *http.Client) *Client {
	return &Client{
		apiURL:        strings.TrimSuffix(apiURL, "/"),
		appID:         appID,
		key:           key,
		http:          httpClient,
		now:           time.Now,
		installations: map[string]int64{},
		tokens:        map[int64]cachedToken{},
	}
}

var (
	defaultMu     sync.Mutex
	defaultClient *Client
)

// Enabled reports whether the integration is switched on and has credentials.
func Enabled() bool {
	return config.GithubEnabled.GetBool() &&
		config.GithubAppID.GetString() != "" &&
		config.GithubPrivateKeyPath.GetString() != ""
}

// Default returns the process-wide client built from config, loading the key once.
func Default() (*Client, error) {
	if !Enabled() {
		return nil, ErrNotConfigured
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultClient != nil {
		return defaultClient, nil
	}
	pem, err := os.ReadFile(config.GithubPrivateKeyPath.GetString())
	if err != nil {
		return nil, fmt.Errorf("read github app private key: %w", err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pem)
	if err != nil {
		return nil, fmt.Errorf("parse github app private key: %w", err)
	}
	defaultClient = NewClient(config.GithubAPIURL.GetString(), config.GithubAppID.GetString(), key, utils.NewUnguardedHTTPClient())
	return defaultClient, nil
}

// SetDefaultForTests swaps the process-wide client; pass nil to reset.
func SetDefaultForTests(c *Client) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultClient = c
}

// appJWT signs the short-lived JWT GitHub requires to act as the App itself.
func (c *Client) appJWT() (string, error) {
	now := c.now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)), // clock drift allowance
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
		Issuer:    c.appID,
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(c.key)
}

// installationToken returns a (cached) installation access token for owner/repo.
func (c *Client) installationToken(ctx context.Context, owner, repo string) (string, error) {
	fullName := strings.ToLower(owner + "/" + repo)

	c.mu.Lock()
	installationID, known := c.installations[fullName]
	if known {
		if t, ok := c.tokens[installationID]; ok && c.now().Before(t.expires.Add(-2*time.Minute)) {
			c.mu.Unlock()
			return t.token, nil
		}
	}
	c.mu.Unlock()

	appToken, err := c.appJWT()
	if err != nil {
		return "", err
	}

	if !known {
		var installation struct {
			ID int64 `json:"id"`
		}
		err = c.request(ctx, http.MethodGet, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/installation", "Bearer "+appToken, nil, &installation)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return "", ErrRepoNotAccessible
		}
		if err != nil {
			return "", err
		}
		installationID = installation.ID
	}

	var token struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	err = c.request(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", installationID), "Bearer "+appToken, nil, &token)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.installations[fullName] = installationID
	c.tokens[installationID] = cachedToken{token: token.Token, expires: token.ExpiresAt}
	c.mu.Unlock()
	return token.Token, nil
}

// repoRequest performs an authenticated call scoped to one repository.
func (c *Client) repoRequest(ctx context.Context, method, owner, repo, path string, body, out any) error {
	token, err := c.installationToken(ctx, owner, repo)
	if err != nil {
		return err
	}
	fullPath := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + path
	return c.request(ctx, method, fullPath, "token "+token, body, out)
}

func (c *Client) request(ctx context.Context, method, path, authorization string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.apiURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "webpart-tasks")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var problem struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &problem) // best effort: the status code alone is enough
		return &APIError{Status: resp.StatusCode, Message: problem.Message}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

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

package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/modules/keyvalue"
)

// ValidSignature checks GitHub's X-Hub-Signature-256 header ("sha256=<hex hmac>") in constant time.
func ValidSignature(secret string, body []byte, header string) bool {
	given, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	givenBytes, err := hex.DecodeString(given)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(givenBytes, mac.Sum(nil))
}

// DeploymentStatus is the part of a deployment_status event the integration uses.
type DeploymentStatus struct {
	ID          int64
	State       string
	Environment string
	Ref         string
	SHA         string
	Owner       string
	Repo        string
	LogURL      string
	TargetURL   string
	EnvURL      string
}

// ParseDeploymentStatus extracts the fields of a deployment_status webhook payload.
func ParseDeploymentStatus(body []byte) (*DeploymentStatus, error) {
	var payload struct {
		DeploymentStatus struct {
			State          string `json:"state"`
			Environment    string `json:"environment"`
			LogURL         string `json:"log_url"`
			TargetURL      string `json:"target_url"`
			EnvironmentURL string `json:"environment_url"`
		} `json:"deployment_status"`
		Deployment struct {
			ID          int64  `json:"id"`
			Ref         string `json:"ref"`
			SHA         string `json:"sha"`
			Environment string `json:"environment"`
		} `json:"deployment"`
		Repository struct {
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	env := payload.DeploymentStatus.Environment
	if env == "" {
		env = payload.Deployment.Environment
	}
	return &DeploymentStatus{
		ID:          payload.Deployment.ID,
		State:       payload.DeploymentStatus.State,
		Environment: env,
		Ref:         payload.Deployment.Ref,
		SHA:         payload.Deployment.SHA,
		Owner:       payload.Repository.Owner.Login,
		Repo:        payload.Repository.Name,
		LogURL:      payload.DeploymentStatus.LogURL,
		TargetURL:   payload.DeploymentStatus.TargetURL,
		EnvURL:      payload.DeploymentStatus.EnvironmentURL,
	}, nil
}

// EnvironmentLabel is "staging" or "production" for the environments that get comments, "" otherwise.
func (d *DeploymentStatus) EnvironmentLabel() string {
	env := strings.ToLower(d.Environment)
	switch {
	case env == "production" || strings.HasPrefix(env, "production"):
		return "production"
	case env == "staging" || strings.HasPrefix(env, "staging"):
		return "staging"
	}
	return ""
}

// Relevant reports whether this status should produce comments: a successful staging or
// production deployment of a known repository.
func (d *DeploymentStatus) Relevant() bool {
	return d.State == "success" && d.EnvironmentLabel() != "" && d.Owner != "" && d.Repo != "" && d.ID != 0
}

// CollectKeys gathers what a deployment covers: task identifiers (HS-334) from the deployed
// ref, the commit message and the pull requests containing the commit (title + branch), and
// bare task numbers from branch names ("feat/335-x"), which match tasks of the linked projects.
// GitHub lookups that fail are logged and skipped; the ref alone may still name a task.
func (d *DeploymentStatus) CollectKeys(ctx context.Context, client *Client) (keys []string, numbers []int64, pulls []*PullRequest) {
	texts := []string{d.Ref}
	refs := []string{d.Ref}
	if d.SHA != "" {
		if msg, err := client.CommitMessage(ctx, d.Owner, d.Repo, d.SHA); err != nil {
			log.Warningf("GitHub deployment %d: could not read commit %s: %s", d.ID, d.SHA, err)
		} else {
			texts = append(texts, msg)
		}
		if found, err := client.PullsForCommit(ctx, d.Owner, d.Repo, d.SHA); err != nil {
			log.Warningf("GitHub deployment %d: could not list pull requests for %s: %s", d.ID, d.SHA, err)
		} else {
			pulls = found
			for _, p := range found {
				texts = append(texts, p.Title, p.HeadRef)
				refs = append(refs, p.HeadRef)
			}
		}
	}
	seen := map[string]bool{}
	for _, text := range texts {
		for _, key := range KeysInText(text) {
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}
	seenNumbers := map[int64]bool{}
	for _, ref := range refs {
		for _, n := range NumbersInRef(ref) {
			if !seenNumbers[n] {
				seenNumbers[n] = true
				numbers = append(numbers, n)
			}
		}
	}
	return keys, numbers, pulls
}

// CommentHTML is the task comment for this deployment.
func (d *DeploymentStatus) CommentHTML(pulls []*PullRequest) string {
	var b strings.Builder
	emoji := "🧪"
	if d.EnvironmentLabel() == "production" {
		emoji = "🚀"
	}
	fmt.Fprintf(&b, "<p>%s Deployed to <strong>%s</strong>", emoji, html.EscapeString(d.EnvironmentLabel()))
	if d.Ref != "" {
		fmt.Fprintf(&b, " from <code>%s</code>", html.EscapeString(d.Ref))
	}
	fmt.Fprintf(&b, " in %s", html.EscapeString(d.Owner+"/"+d.Repo))
	if len(pulls) > 0 {
		p := pulls[0]
		fmt.Fprintf(&b, " · <a href=\"%s\">#%d %s</a>", html.EscapeString(p.URL), p.Number, html.EscapeString(p.Title))
	}
	runURL := d.LogURL
	if runURL == "" {
		runURL = d.TargetURL
	}
	if runURL != "" {
		fmt.Fprintf(&b, " · <a href=\"%s\">run</a>", html.EscapeString(runURL))
	}
	if d.EnvURL != "" {
		fmt.Fprintf(&b, " · <a href=\"%s\">open</a>", html.EscapeString(d.EnvURL))
	}
	b.WriteString("</p>")
	return b.String()
}

// MarkDeploymentCommented records that a task got this deployment's comment and reports
// whether it is the first time (GitHub redelivers webhooks; each task is commented once).
func MarkDeploymentCommented(d *DeploymentStatus, taskID int64) bool {
	key := fmt.Sprintf("github-deploy-%d-%s-%d", d.ID, d.EnvironmentLabel(), taskID)
	n, err := keyvalue.IncrByWithTTL(key, 1, 30*24*time.Hour)
	if err != nil {
		log.Errorf("GitHub deployment %d: could not record comment for task %d: %s", d.ID, taskID, err)
		return true // better a duplicate comment than a missing one
	}
	return n == 1
}

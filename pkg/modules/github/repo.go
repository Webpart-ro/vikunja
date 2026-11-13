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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/config"
)

// CIStatus summarises the check runs on one commit.
type CIStatus struct {
	// success, failure, pending or neutral; empty when the commit has no checks.
	State string `json:"state" doc:"Overall CI state of the commit: success, failure, pending or neutral. Empty when it has no checks."`
	URL   string `json:"url,omitempty" doc:"Link to the most relevant run (the failing one if any) on GitHub."`
	Total int    `json:"total" doc:"How many check runs the commit has."`
}

// Branch is a branch that mentions the task.
type Branch struct {
	Name string    `json:"name" doc:"The branch name."`
	URL  string    `json:"url" doc:"Link to the branch on GitHub."`
	SHA  string    `json:"sha" doc:"The commit the branch points to."`
	CI   *CIStatus `json:"ci,omitempty" doc:"CI status of the branch head."`
}

// PullRequest is a pull request that mentions the task.
type PullRequest struct {
	Number    int64     `json:"number" doc:"The pull request number."`
	Title     string    `json:"title" doc:"The pull request title."`
	URL       string    `json:"url" doc:"Link to the pull request on GitHub."`
	State     string    `json:"state" doc:"open, merged or closed."`
	Draft     bool      `json:"draft" doc:"Whether the pull request is a draft."`
	HeadRef   string    `json:"head_ref" doc:"The source branch."`
	BaseRef   string    `json:"base_ref" doc:"The target branch."`
	HeadSHA   string    `json:"-"`
	Author    string    `json:"author" doc:"GitHub login of the author."`
	UpdatedAt time.Time `json:"updated_at" doc:"When the pull request last changed."`
	CI        *CIStatus `json:"ci,omitempty" doc:"CI status of the pull request head."`
}

// RepoDevelopment is everything one repository has for one task.
type RepoDevelopment struct {
	FullName     string         `json:"full_name" doc:"owner/repo."`
	URL          string         `json:"url" doc:"Link to the repository."`
	Accessible   bool           `json:"accessible" doc:"False when the GitHub App can't read this repository (not installed on it)."`
	Branches     []*Branch      `json:"branches" doc:"Branches whose name mentions the task identifier."`
	PullRequests []*PullRequest `json:"pull_requests" doc:"Pull requests whose title, description or branch mentions the task identifier."`
}

// keyPattern matches a task identifier such as HS-334 as a whole token, case-insensitively,
// so HS-334 doesn't match HS-3345 or XHS-334.
func keyPattern(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(^|[^A-Za-z0-9])` + regexp.QuoteMeta(key) + `($|[^0-9])`)
}

// MentionsKey reports whether text mentions the task identifier.
func MentionsKey(text, key string) bool {
	if key == "" || strings.HasPrefix(key, "#") {
		return false
	}
	return keyPattern(key).MatchString(text)
}

// Number-only references. Tasks imported from GitHub issues share their number
// (HS-335 is hunting-store.ro#335), so within a linked repository a branch named
// "feat/335-checkout" or a pull request saying "Closes #335" also belongs to the task.

// numberFromKey returns 335 for "HS-335", 0 when key has no number.
func numberFromKey(key string) int64 {
	dash := strings.LastIndex(key, "-")
	if dash < 0 {
		return 0
	}
	n, err := strconv.ParseInt(key[dash+1:], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// refNumberPattern: a path segment of a branch name that starts with the number followed by
// '-' or the end: "feat/335-checkout", "335", "fix/335". Not "feat/3350-x" or "v2-335".
func refNumberPattern(n int64) *regexp.Regexp {
	return regexp.MustCompile(`(^|/)` + strconv.FormatInt(n, 10) + `($|-|/)`)
}

// RefMentionsNumber reports whether a branch name refers to issue/task number n.
func RefMentionsNumber(ref string, n int64) bool {
	return n > 0 && refNumberPattern(n).MatchString(ref)
}

// TitleMentionsNumber reports whether a pull request title refers to #n, e.g.
// "feat(feeds): supplier feed checks (#107)", ignoring cross-repository refs.
func TitleMentionsNumber(title string, n int64) bool {
	if n <= 0 {
		return false
	}
	return regexp.MustCompile(`(^|[^A-Za-z0-9/_.-])#` + strconv.FormatInt(n, 10) + `($|[^0-9])`).MatchString(title)
}

// BodyMentionsNumber reports whether a pull request description links #n with a keyword:
// GitHub's closing keywords plus "refs" and "part of" ("Closes #335", "Refs #335").
// Passing mentions ("groundwork for #335") don't count; they made every related PR show up.
func BodyMentionsNumber(body string, n int64) bool {
	if n <= 0 {
		return false
	}
	return regexp.MustCompile(`(?i)\b(close[sd]?|fix(e[sd])?|resolve[sd]?|refs?|references|part of|implements?)\b:?\s+#` +
		strconv.FormatInt(n, 10) + `($|[^0-9])`).MatchString(body)
}

var leadingNumberSegment = regexp.MustCompile(`^([0-9]{1,9})($|-)`)

// NumbersInRef returns the numbers branch-name segments start with ("feat/335-x" -> [335]).
func NumbersInRef(ref string) []int64 {
	numbers := []int64{}
	for _, segment := range strings.Split(ref, "/") {
		if m := leadingNumberSegment.FindStringSubmatch(segment); m != nil {
			if n, err := strconv.ParseInt(m[1], 10, 64); err == nil && n > 0 {
				numbers = append(numbers, n)
			}
		}
	}
	return numbers
}

// keyInText finds every task identifier in text, uppercased, e.g. "fix HS-12, hs-3" -> [HS-12 HS-3].
var anyKeyPattern = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])([A-Za-z][A-Za-z0-9]{0,9}-[0-9]{1,9})(?:$|[^0-9])`)

// KeysInText returns the distinct task identifiers mentioned in text, uppercased.
func KeysInText(text string) []string {
	seen := map[string]bool{}
	keys := []string{}
	// FindAll can't return overlapping matches; separators are shared between neighbours, so scan
	// with a moving offset.
	for rest := text; rest != ""; {
		loc := anyKeyPattern.FindStringSubmatchIndex(rest)
		if loc == nil {
			break
		}
		key := strings.ToUpper(rest[loc[2]:loc[3]])
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
		rest = rest[loc[3]:]
	}
	return keys
}

type ghBranch struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

type ghPull struct {
	Number   int64     `json:"number"`
	Title    string    `json:"title"`
	Body     string    `json:"body"`
	HTMLURL  string    `json:"html_url"`
	State    string    `json:"state"`
	Draft    bool      `json:"draft"`
	MergedAt *string   `json:"merged_at"`
	Updated  time.Time `json:"updated_at"`
	User     struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func (p *ghPull) toPullRequest() *PullRequest {
	state := p.State
	if p.MergedAt != nil && *p.MergedAt != "" {
		state = "merged"
	}
	return &PullRequest{
		Number: p.Number, Title: p.Title, URL: p.HTMLURL, State: state, Draft: p.Draft,
		HeadRef: p.Head.Ref, BaseRef: p.Base.Ref, HeadSHA: p.Head.SHA, Author: p.User.Login, UpdatedAt: p.Updated,
	}
}

const (
	maxBranchPages = 3 // 300 branches
	maxPullPages   = 2 // 200 most recently updated pull requests
	maxCIChecks    = 8 // CI lookups per repository per request
)

func repoHTMLURL(owner, repo string) string {
	return "https://github.com/" + owner + "/" + repo
}

func (c *Client) listBranches(ctx context.Context, owner, repo string) ([]ghBranch, error) {
	all := []ghBranch{}
	for page := 1; page <= maxBranchPages; page++ {
		batch := []ghBranch{}
		if err := c.repoRequest(ctx, http.MethodGet, owner, repo, fmt.Sprintf("/branches?per_page=100&page=%d", page), nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
	}
	return all, nil
}

func (c *Client) listPulls(ctx context.Context, owner, repo string) ([]ghPull, error) {
	all := []ghPull{}
	for page := 1; page <= maxPullPages; page++ {
		batch := []ghPull{}
		path := fmt.Sprintf("/pulls?state=all&sort=updated&direction=desc&per_page=100&page=%d", page)
		if err := c.repoRequest(ctx, http.MethodGet, owner, repo, path, nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
	}
	return all, nil
}

// CI returns the combined check-run state of a commit.
func (c *Client) CI(ctx context.Context, owner, repo, sha string) (*CIStatus, error) {
	var result struct {
		Total     int `json:"total_count"`
		CheckRuns []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
		} `json:"check_runs"`
	}
	if err := c.repoRequest(ctx, http.MethodGet, owner, repo, "/commits/"+url.PathEscape(sha)+"/check-runs?per_page=50", nil, &result); err != nil {
		return nil, err
	}
	status := &CIStatus{Total: result.Total}
	if len(result.CheckRuns) == 0 {
		return status, nil
	}
	pending, failed := false, false
	status.URL = result.CheckRuns[0].HTMLURL
	for _, run := range result.CheckRuns {
		switch {
		case run.Status != "completed":
			pending = true
		case run.Conclusion == "failure" || run.Conclusion == "timed_out" || run.Conclusion == "cancelled" || run.Conclusion == "action_required" || run.Conclusion == "startup_failure":
			if !failed {
				status.URL = run.HTMLURL
			}
			failed = true
		}
	}
	switch {
	case failed:
		status.State = "failure"
	case pending:
		status.State = "pending"
	default:
		status.State = "success"
		for _, run := range result.CheckRuns {
			if run.Conclusion != "success" && run.Conclusion != "skipped" && run.Conclusion != "neutral" {
				status.State = "neutral"
			}
		}
	}
	return status, nil
}

// Development collects the branches and pull requests of owner/repo that mention key,
// with CI status for the open ones.
func (c *Client) Development(ctx context.Context, owner, repo, key string) (*RepoDevelopment, error) {
	dev := &RepoDevelopment{FullName: owner + "/" + repo, URL: repoHTMLURL(owner, repo), Accessible: true, Branches: []*Branch{}, PullRequests: []*PullRequest{}}

	branches, err := c.listBranches(ctx, owner, repo)
	if errors.Is(err, ErrRepoNotAccessible) {
		dev.Accessible = false
		return dev, nil
	}
	if err != nil {
		return nil, err
	}
	number := numberFromKey(key)
	for _, b := range branches {
		if MentionsKey(b.Name, key) || RefMentionsNumber(b.Name, number) {
			dev.Branches = append(dev.Branches, &Branch{Name: b.Name, SHA: b.Commit.SHA, URL: repoHTMLURL(owner, repo) + "/tree/" + b.Name})
		}
	}

	pulls, err := c.listPulls(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	for i := range pulls {
		p := &pulls[i]
		if MentionsKey(p.Title, key) || MentionsKey(p.Head.Ref, key) || MentionsKey(p.Body, key) ||
			RefMentionsNumber(p.Head.Ref, number) || TitleMentionsNumber(p.Title, number) || BodyMentionsNumber(p.Body, number) {
			dev.PullRequests = append(dev.PullRequests, p.toPullRequest())
		}
	}
	// Open first, then most recently updated.
	sort.SliceStable(dev.PullRequests, func(i, j int) bool {
		a, b := dev.PullRequests[i], dev.PullRequests[j]
		if (a.State == "open") != (b.State == "open") {
			return a.State == "open"
		}
		return a.UpdatedAt.After(b.UpdatedAt)
	})

	checks := 0
	for _, p := range dev.PullRequests {
		if p.State != "open" || checks >= maxCIChecks {
			continue
		}
		checks++
		if p.CI, err = c.CI(ctx, owner, repo, p.HeadSHA); err != nil {
			return nil, err
		}
	}
	for _, b := range dev.Branches {
		if checks >= maxCIChecks {
			break
		}
		checks++
		if b.CI, err = c.CI(ctx, owner, repo, b.SHA); err != nil {
			return nil, err
		}
	}
	return dev, nil
}

// BranchNames lists the repository's branches (for picking a base branch) and its default branch.
func (c *Client) BranchNames(ctx context.Context, owner, repo string) (names []string, defaultBranch string, err error) {
	branches, err := c.listBranches(ctx, owner, repo)
	if err != nil {
		return nil, "", err
	}
	names = make([]string, 0, len(branches))
	for _, b := range branches {
		names = append(names, b.Name)
	}
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.repoRequest(ctx, http.MethodGet, owner, repo, "", nil, &info); err != nil {
		return nil, "", err
	}
	return names, info.DefaultBranch, nil
}

// ErrBranchExists is returned when the branch to create already exists.
var ErrBranchExists = errors.New("branch already exists")

// ErrBaseBranchNotFound is returned when the base branch doesn't exist.
var ErrBaseBranchNotFound = errors.New("base branch not found")

// validBranchName rejects names git refuses (subset of git check-ref-format).
var validBranchName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

// ValidBranchName reports whether name is an acceptable new branch name.
func ValidBranchName(name string) bool {
	return validBranchName.MatchString(name) &&
		!strings.Contains(name, "..") && !strings.Contains(name, "//") &&
		!strings.HasSuffix(name, "/") && !strings.HasSuffix(name, ".") && !strings.HasSuffix(name, ".lock")
}

// CreateBranch creates name from base in owner/repo. An empty base uses the configured
// default (develop), falling back to the repository's default branch if that doesn't exist.
func (c *Client) CreateBranch(ctx context.Context, owner, repo, base, name string) (*Branch, error) {
	explicitBase := base != ""
	if !explicitBase {
		base = config.GithubDefaultBase.GetString()
	}

	sha, err := c.refSHA(ctx, owner, repo, base)
	if errors.Is(err, ErrBaseBranchNotFound) && !explicitBase {
		var info struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := c.repoRequest(ctx, http.MethodGet, owner, repo, "", nil, &info); err != nil {
			return nil, err
		}
		sha, err = c.refSHA(ctx, owner, repo, info.DefaultBranch)
	}
	if err != nil {
		return nil, err
	}

	body := map[string]string{"ref": "refs/heads/" + name, "sha": sha}
	err = c.repoRequest(ctx, http.MethodPost, owner, repo, "/git/refs", body, nil)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity {
		return nil, ErrBranchExists
	}
	if err != nil {
		return nil, err
	}
	return &Branch{Name: name, SHA: sha, URL: repoHTMLURL(owner, repo) + "/tree/" + name}, nil
}

func (c *Client) refSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	err := c.repoRequest(ctx, http.MethodGet, owner, repo, "/git/ref/heads/"+branch, nil, &ref)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return "", ErrBaseBranchNotFound
	}
	return ref.Object.SHA, err
}

// CommitMessage returns the message of a commit.
func (c *Client) CommitMessage(ctx context.Context, owner, repo, sha string) (string, error) {
	var commit struct {
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	err := c.repoRequest(ctx, http.MethodGet, owner, repo, "/commits/"+url.PathEscape(sha), nil, &commit)
	return commit.Commit.Message, err
}

// PullsForCommit returns the pull requests that contain a commit.
func (c *Client) PullsForCommit(ctx context.Context, owner, repo, sha string) ([]*PullRequest, error) {
	pulls := []ghPull{}
	if err := c.repoRequest(ctx, http.MethodGet, owner, repo, "/commits/"+url.PathEscape(sha)+"/pulls", nil, &pulls); err != nil {
		return nil, err
	}
	out := make([]*PullRequest, 0, len(pulls))
	for i := range pulls {
		out = append(out, pulls[i].toPullRequest())
	}
	return out, nil
}

const developmentCacheTTL = time.Minute

type cachedDevelopment struct {
	dev     *RepoDevelopment
	expires time.Time
}

func developmentCacheKey(owner, repo, key string) string {
	return strings.ToLower(owner + "/" + repo + "#" + key)
}

// DevelopmentCached is Development with a short in-memory cache, so opening a task
// repeatedly doesn't hit GitHub every time.
func (c *Client) DevelopmentCached(ctx context.Context, owner, repo, key string) (*RepoDevelopment, error) {
	cacheKey := developmentCacheKey(owner, repo, key)
	c.mu.Lock()
	if c.devCache == nil {
		c.devCache = map[string]cachedDevelopment{}
	}
	if hit, ok := c.devCache[cacheKey]; ok && c.now().Before(hit.expires) {
		c.mu.Unlock()
		return hit.dev, nil
	}
	c.mu.Unlock()

	dev, err := c.Development(ctx, owner, repo, key)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.devCache[cacheKey] = cachedDevelopment{dev: dev, expires: c.now().Add(developmentCacheTTL)}
	c.mu.Unlock()
	return dev, nil
}

// ForgetDevelopment drops the cached data for one task in one repository (after creating a branch).
func (c *Client) ForgetDevelopment(owner, repo, key string) {
	c.mu.Lock()
	delete(c.devCache, developmentCacheKey(owner, repo, key))
	c.mu.Unlock()
}

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

package apiv2

// Webpart GitHub integration: project <-> repository links, the "Development" data of a
// task (branches, pull requests, CI that mention its identifier), creating a branch from a
// task, and the GitHub App webhook that comments on tasks when they reach staging/production.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/modules/github"
	"code.vikunja.io/api/pkg/web"
	"code.vikunja.io/api/pkg/web/handler"

	"github.com/danielgtaylor/huma/v2"
	"xorm.io/xorm"
)

type githubRepoListBody struct {
	Body Paginated[*models.ProjectGithubRepo]
}

type taskDevelopmentBody struct {
	// The task identifier the GitHub data was matched on, e.g. HS-334.
	Key          string                    `json:"key" doc:"The task identifier branches and pull requests are matched on, e.g. HS-334. Empty when the project has no identifier."`
	Configured   bool                      `json:"configured" doc:"Whether the GitHub integration is configured on this instance."`
	Repositories []*github.RepoDevelopment `json:"repositories" doc:"One entry per repository linked to the task's project."`
}

type branchBasesBody struct {
	Branches      []string `json:"branches" doc:"The repository's branches."`
	DefaultBase   string   `json:"default_base" doc:"The branch preselected as base: the configured default (develop) if the repository has it, otherwise its default branch."`
	DefaultBranch string   `json:"default_branch" doc:"The repository's default branch on GitHub."`
}

type createBranchInput struct {
	Repo string `json:"repo" doc:"owner/repo; must be linked to the task's project."`
	Base string `json:"base,omitempty" doc:"Branch to start from. Empty uses the configured default (develop), falling back to the repository's default branch."`
	Name string `json:"name" doc:"The new branch name. Must mention the task identifier so the branch shows up on the task."`
}

// RegisterGithubRoutes wires the GitHub integration onto the Huma API. Everything is gated by
// github.enabled; the webhook is public (GitHub calls it) and authenticated by its HMAC signature.
func RegisterGithubRoutes(api huma.API) {
	if !config.GithubEnabled.GetBool() {
		return
	}
	tags := []string{"github"}

	Register(api, huma.Operation{
		OperationID: "project-github-repos-list",
		Summary:     "List a project's linked GitHub repositories",
		Description: "Returns the GitHub repositories whose branches and pull requests are matched against this project's tasks. Requires read access to the project.",
		Method:      http.MethodGet, Path: "/projects/{project}/github/repos", Tags: tags,
	}, githubReposList)
	Register(api, huma.Operation{
		OperationID: "project-github-repos-create",
		Summary:     "Link a GitHub repository to a project",
		Description: "Links owner/repo to the project. Requires write access to the project. The GitHub App must be installed on the repository for data to show up; linking itself does not call GitHub.",
		Method:      http.MethodPost, Path: "/projects/{project}/github/repos", Tags: tags,
	}, githubReposCreate)
	Register(api, huma.Operation{
		OperationID: "project-github-repos-delete",
		Summary:     "Unlink a GitHub repository",
		Description: "Removes the link. Nothing changes on GitHub. Requires write access to the project.",
		Method:      http.MethodDelete, Path: "/projects/{project}/github/repos/{repo}", Tags: tags,
	}, githubReposDelete)

	Register(api, huma.Operation{
		OperationID: "task-development-read",
		Summary:     "Get a task's branches, pull requests and CI",
		Description: "Searches the repositories linked to the task's project for branches whose name, and pull requests whose title, description or branch mention the task identifier (e.g. HS-334, case-insensitive), with the CI status of open ones. Results are live from GitHub, cached for about a minute. Requires read access to the task.",
		Method:      http.MethodGet, Path: "/tasks/{id}/development", Tags: tags,
	}, taskDevelopmentRead)
	Register(api, huma.Operation{
		OperationID: "task-development-bases",
		Summary:     "List base branches for a new branch",
		Description: "Lists the branches of a linked repository to start a new branch from. Requires write access to the task.",
		Method:      http.MethodGet, Path: "/tasks/{id}/development/bases", Tags: tags,
	}, taskDevelopmentBases)
	Register(api, huma.Operation{
		OperationID: "task-development-branch-create",
		Summary:     "Create a GitHub branch for a task",
		Description: "Creates a branch on GitHub in a repository linked to the task's project. The name must mention the task identifier. Anyone who can edit the task may do this. 409 if the branch exists, 400 if the base branch doesn't.",
		Method:      http.MethodPost, Path: "/tasks/{id}/development/branches", Tags: tags,
	}, taskDevelopmentCreateBranch)

	Register(api, huma.Operation{
		OperationID: "github-webhook",
		Summary:     "GitHub App webhook",
		Description: "Receives GitHub App events. Requests must carry a valid X-Hub-Signature-256 for github.webhooksecret. Successful deployment_status events for the staging and production environments add a comment to every task mentioned by the deployed branch, its pull request or commit message (once per deployment and task). Other events are acknowledged and ignored.",
		Method:      http.MethodPost, Path: "/integrations/github/webhook", Tags: tags,
		Security:      []map[string][]string{},
		DefaultStatus: http.StatusAccepted,
	}, githubWebhook)
}

func init() { AddRouteRegistrar(RegisterGithubRoutes) }

// ---- project repository links ----

func githubReposList(ctx context.Context, in *struct {
	ProjectID int64 `path:"project"`
	ListParams
}) (*githubRepoListBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	result, _, total, err := handler.DoReadAll(ctx, &models.ProjectGithubRepo{ProjectID: in.ProjectID}, a, in.Q, in.Page, in.PerPage)
	if err != nil {
		return nil, translateDomainError(err)
	}
	items, ok := result.([]*models.ProjectGithubRepo)
	if !ok {
		return nil, fmt.Errorf("ProjectGithubRepo.ReadAll returned unexpected type %T", result)
	}
	return &githubRepoListBody{Body: NewPaginated(items, total, in.Page, in.PerPage)}, nil
}

func githubReposCreate(ctx context.Context, in *struct {
	ProjectID int64 `path:"project"`
	Body      models.ProjectGithubRepo
}) (*singleBody[models.ProjectGithubRepo], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	in.Body.ProjectID = in.ProjectID // URL wins over body
	if err := handler.DoCreate(ctx, &in.Body, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.ProjectGithubRepo]{Body: &in.Body}, nil
}

func githubReposDelete(ctx context.Context, in *struct {
	ProjectID int64 `path:"project"`
	ID        int64 `path:"repo"`
}) (*emptyBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := handler.DoDelete(ctx, &models.ProjectGithubRepo{ID: in.ID, ProjectID: in.ProjectID}, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &emptyBody{}, nil
}

// ---- task development data ----

// taskWithKey loads a task the caller may read (or update, when write is true) plus its
// identifier and its project's linked repositories.
func taskWithKey(ctx context.Context, taskID int64, write bool) (key string, repos []*models.ProjectGithubRepo, err error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return "", nil, err
	}
	s := db.NewSession()
	defer s.Close()

	task := &models.Task{ID: taskID}
	var can bool
	if write {
		can, err = task.CanUpdate(s, a)
	} else {
		can, _, err = task.CanRead(s, a)
	}
	if err != nil {
		return "", nil, translateDomainError(err)
	}
	if !can {
		return "", nil, huma.Error403Forbidden("forbidden")
	}

	full, err := models.GetTaskByIDSimple(s, taskID)
	if err != nil {
		return "", nil, translateDomainError(err)
	}
	project, err := models.GetProjectSimpleByID(s, full.ProjectID)
	if err != nil {
		return "", nil, translateDomainError(err)
	}
	if project.Identifier != "" {
		key = fmt.Sprintf("%s-%d", project.Identifier, full.Index)
	}
	repos, err = models.GetGithubReposForProject(s, full.ProjectID)
	if err != nil {
		return "", nil, translateDomainError(err)
	}
	return key, repos, nil
}

func githubError(err error) error {
	var apiErr *github.APIError
	switch {
	case errors.Is(err, github.ErrNotConfigured):
		return huma.Error503ServiceUnavailable("The GitHub integration is not configured.")
	case errors.Is(err, github.ErrRepoNotAccessible):
		return huma.Error400BadRequest("The GitHub App is not installed on this repository.")
	case errors.Is(err, github.ErrBranchExists):
		return huma.Error409Conflict("A branch with this name already exists.")
	case errors.Is(err, github.ErrBaseBranchNotFound):
		return huma.Error400BadRequest("The base branch does not exist.")
	case errors.As(err, &apiErr):
		log.Errorf("GitHub API error: %s", apiErr)
		return huma.Error502BadGateway("GitHub returned an error.")
	}
	return err
}

func taskDevelopmentRead(ctx context.Context, in *struct {
	ID int64 `path:"id"`
}) (*singleBody[taskDevelopmentBody], error) {
	key, repos, err := taskWithKey(ctx, in.ID, false)
	if err != nil {
		return nil, err
	}
	body := &taskDevelopmentBody{Key: key, Configured: github.Enabled(), Repositories: []*github.RepoDevelopment{}}
	if !body.Configured || key == "" || len(repos) == 0 {
		return &singleBody[taskDevelopmentBody]{Body: body}, nil
	}

	client, err := github.Default()
	if err != nil {
		return nil, githubError(err)
	}
	for _, r := range repos {
		dev, err := client.DevelopmentCached(ctx, r.Owner, r.Repo, key)
		if err != nil {
			return nil, githubError(err)
		}
		body.Repositories = append(body.Repositories, dev)
	}
	return &singleBody[taskDevelopmentBody]{Body: body}, nil
}

// linkedRepo finds owner/repo among the task project's linked repositories.
func linkedRepo(repos []*models.ProjectGithubRepo, fullName string) *models.ProjectGithubRepo {
	for _, r := range repos {
		if strings.EqualFold(r.FullName(), strings.TrimSpace(fullName)) {
			return r
		}
	}
	return nil
}

func taskDevelopmentBases(ctx context.Context, in *struct {
	ID   int64  `path:"id"`
	Repo string `query:"repo" doc:"owner/repo; must be linked to the task's project."`
}) (*singleBody[branchBasesBody], error) {
	_, repos, err := taskWithKey(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	repo := linkedRepo(repos, in.Repo)
	if repo == nil {
		return nil, huma.Error400BadRequest("This repository is not linked to the task's project.")
	}
	client, err := github.Default()
	if err != nil {
		return nil, githubError(err)
	}
	names, defaultBranch, err := client.BranchNames(ctx, repo.Owner, repo.Repo)
	if err != nil {
		return nil, githubError(err)
	}
	body := &branchBasesBody{Branches: names, DefaultBranch: defaultBranch, DefaultBase: defaultBranch}
	preferred := config.GithubDefaultBase.GetString()
	for _, n := range names {
		if n == preferred {
			body.DefaultBase = preferred
		}
	}
	return &singleBody[branchBasesBody]{Body: body}, nil
}

func taskDevelopmentCreateBranch(ctx context.Context, in *struct {
	ID   int64 `path:"id"`
	Body createBranchInput
}) (*singleBody[github.Branch], error) {
	key, repos, err := taskWithKey(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, huma.Error400BadRequest("The task's project has no identifier, so branches can't be linked to it.")
	}
	repo := linkedRepo(repos, in.Body.Repo)
	if repo == nil {
		return nil, huma.Error400BadRequest("This repository is not linked to the task's project.")
	}
	name := strings.TrimSpace(in.Body.Name)
	if !github.ValidBranchName(name) {
		return nil, huma.Error400BadRequest("Invalid branch name.")
	}
	if !github.MentionsKey(name, key) {
		return nil, huma.Error400BadRequest(fmt.Sprintf("The branch name must contain %s.", key))
	}
	client, err := github.Default()
	if err != nil {
		return nil, githubError(err)
	}
	branch, err := client.CreateBranch(ctx, repo.Owner, repo.Repo, strings.TrimSpace(in.Body.Base), name)
	if err != nil {
		return nil, githubError(err)
	}
	client.ForgetDevelopment(repo.Owner, repo.Repo, key)
	return &singleBody[github.Branch]{Body: branch}, nil
}

// ---- webhook ----

type githubWebhookInput struct {
	Event     string `header:"X-GitHub-Event"`
	Signature string `header:"X-Hub-Signature-256"`
	Delivery  string `header:"X-GitHub-Delivery"`
	RawBody   []byte
}

type githubWebhookResult struct {
	Body struct {
		Handled  bool    `json:"handled" doc:"Whether the event led to any action."`
		Comments int     `json:"comments" doc:"How many task comments were added."`
		TaskIDs  []int64 `json:"task_ids,omitempty" doc:"The tasks that got a comment."`
	}
}

func githubWebhook(ctx context.Context, in *githubWebhookInput) (*githubWebhookResult, error) {
	secret := config.GithubWebhookSecret.GetString()
	if secret == "" || !github.ValidSignature(secret, in.RawBody, in.Signature) {
		return nil, huma.Error401Unauthorized("invalid signature")
	}
	out := &githubWebhookResult{}
	if in.Event != "deployment_status" {
		return out, nil
	}

	deployment, err := github.ParseDeploymentStatus(in.RawBody)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid deployment_status payload")
	}
	if !deployment.Relevant() {
		return out, nil
	}

	bot := config.GithubBotUsername.GetString()
	if bot == "" {
		log.Debugf("GitHub deployment %d: github.botusername not set, not commenting", deployment.ID)
		return out, nil
	}

	client, err := github.Default()
	if err != nil {
		return nil, githubError(err)
	}
	keys, numbers, pulls := deployment.CollectKeys(ctx, client)
	if len(keys) == 0 && len(numbers) == 0 {
		return out, nil
	}

	taskIDs, err := commentOnDeployedTasks(deployment, keys, numbers, pulls, bot)
	if err != nil {
		return nil, translateDomainError(err)
	}
	out.Body.Handled = len(taskIDs) > 0
	out.Body.Comments = len(taskIDs)
	out.Body.TaskIDs = taskIDs
	return out, nil
}

func commentOnDeployedTasks(d *github.DeploymentStatus, keys []string, numbers []int64, pulls []*github.PullRequest, botUsername string) (taskIDs []int64, err error) {
	s := db.NewSession()
	defer s.Close()
	if err := s.Begin(); err != nil {
		return nil, err
	}

	botUser, err := models.GetUserForGithubComments(s, botUsername)
	if err != nil {
		_ = s.Rollback()
		return nil, err
	}

	projectIDs, err := models.GetProjectsForGithubRepo(s, d.Owner, d.Repo)
	if err != nil {
		_ = s.Rollback()
		return nil, err
	}

	// Candidate tasks: identifiers (HS-334) first, then bare numbers from branch names.
	candidates := []*models.Task{}
	for _, key := range keys {
		task, err := models.FindTaskByIdentifierInProjects(s, key, projectIDs)
		if err != nil {
			_ = s.Rollback()
			return nil, err
		}
		if task != nil {
			candidates = append(candidates, task)
		}
	}
	for _, n := range numbers {
		found, err := models.FindTasksByIndexInProjects(s, n, projectIDs)
		if err != nil {
			_ = s.Rollback()
			return nil, err
		}
		candidates = append(candidates, found...)
	}

	comment := d.CommentHTML(pulls)
	done := map[int64]bool{}
	for _, task := range candidates {
		if done[task.ID] {
			continue
		}
		done[task.ID] = true
		canComment, err := models.CanPostGithubComment(s, botUser, task)
		if err != nil {
			_ = s.Rollback()
			return nil, err
		}
		if !canComment {
			log.Warningf("GitHub deployment %d: %s has no write access to project %d, not commenting on task %d", d.ID, botUsername, task.ProjectID, task.ID)
			continue
		}
		if !github.MarkDeploymentCommented(d, task.ID) {
			continue // already commented on this deployment for this task
		}
		if err := addBotComment(s, task.ID, comment, botUser); err != nil {
			_ = s.Rollback()
			return nil, err
		}
		taskIDs = append(taskIDs, task.ID)
	}

	if err := s.Commit(); err != nil {
		return nil, err
	}
	return taskIDs, nil
}

func addBotComment(s *xorm.Session, taskID int64, html string, bot web.Auth) error {
	tc := &models.TaskComment{TaskID: taskID, Comment: html}
	return tc.Create(s, bot)
}

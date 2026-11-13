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

package models

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/web"

	"xorm.io/builder"
	"xorm.io/xorm"
)

// ProjectGithubRepo links a project to a GitHub repository (Webpart GitHub integration).
// Branches, pull requests and CI runs whose names mention a task's identifier
// (e.g. HS-334) show up on that task.
type ProjectGithubRepo struct {
	ID        int64  `xorm:"autoincr not null unique pk" json:"id" param:"repo" readOnly:"true" doc:"The id of this repository link."`
	ProjectID int64  `xorm:"bigint not null index" json:"project_id" param:"project" readOnly:"true" doc:"The project this repository is linked to. Set from the URL."`
	Owner     string `xorm:"varchar(100) not null" json:"owner" valid:"required" maxLength:"100" doc:"The GitHub user or organization owning the repository, e.g. Webpart-ro."`
	Repo      string `xorm:"varchar(100) not null" json:"repo" valid:"required" maxLength:"100" doc:"The repository name, e.g. hunting-store.ro."`

	CreatedByID int64     `xorm:"bigint not null" json:"-"`
	Created     time.Time `xorm:"created not null" json:"created" readOnly:"true" doc:"When the repository was linked."`

	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

// TableName holds the table name.
func (*ProjectGithubRepo) TableName() string {
	return "project_github_repos"
}

// FullName is owner/repo.
func (r *ProjectGithubRepo) FullName() string {
	return r.Owner + "/" + r.Repo
}

// GitHub owner and repository names: letters, digits, '-', '_' and '.'.
var githubNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// ErrInvalidGithubRepo is returned for an owner/repo that can't be a GitHub repository.
type ErrInvalidGithubRepo struct{}

func (*ErrInvalidGithubRepo) Error() string { return "invalid GitHub repository" }

// ErrCodeInvalidGithubRepo holds the unique world-error code of this error (Webpart range 1090+).
const ErrCodeInvalidGithubRepo = 1092

// HTTPError holds the http error description
func (*ErrInvalidGithubRepo) HTTPError() web.HTTPError {
	return web.HTTPError{
		HTTPCode: http.StatusBadRequest,
		Code:     ErrCodeInvalidGithubRepo,
		Message:  "The GitHub owner or repository name is invalid.",
	}
}

// ErrGithubRepoAlreadyLinked is returned when a project already has this repository.
type ErrGithubRepoAlreadyLinked struct{}

func (*ErrGithubRepoAlreadyLinked) Error() string { return "GitHub repository already linked" }

// ErrCodeGithubRepoAlreadyLinked holds the unique world-error code of this error.
const ErrCodeGithubRepoAlreadyLinked = 1093

// HTTPError holds the http error description
func (*ErrGithubRepoAlreadyLinked) HTTPError() web.HTTPError {
	return web.HTTPError{
		HTTPCode: http.StatusConflict,
		Code:     ErrCodeGithubRepoAlreadyLinked,
		Message:  "This GitHub repository is already linked to the project.",
	}
}

// Create links a repository to the project.
func (r *ProjectGithubRepo) Create(s *xorm.Session, a web.Auth) error {
	r.Owner = strings.TrimSpace(r.Owner)
	r.Repo = strings.TrimSpace(strings.TrimSuffix(r.Repo, ".git"))
	if !githubNamePattern.MatchString(r.Owner) || !githubNamePattern.MatchString(r.Repo) {
		return &ErrInvalidGithubRepo{}
	}

	exists, err := s.Where(builder.Eq{"project_id": r.ProjectID}).
		And("LOWER(owner) = ? AND LOWER(repo) = ?", strings.ToLower(r.Owner), strings.ToLower(r.Repo)).
		Exist(&ProjectGithubRepo{})
	if err != nil {
		return err
	}
	if exists {
		return &ErrGithubRepoAlreadyLinked{}
	}

	r.ID = 0
	r.CreatedByID = a.GetID()
	_, err = s.Insert(r)
	return err
}

// ReadAll returns the repositories linked to the project.
func (r *ProjectGithubRepo) ReadAll(s *xorm.Session, _ web.Auth, _ string, _ int, _ int) (result interface{}, resultCount int, numberOfTotalItems int64, err error) {
	repos, err := GetGithubReposForProject(s, r.ProjectID)
	if err != nil {
		return nil, 0, 0, err
	}
	return repos, len(repos), int64(len(repos)), nil
}

// Delete unlinks the repository.
func (r *ProjectGithubRepo) Delete(s *xorm.Session, _ web.Auth) error {
	_, err := s.Where("id = ? AND project_id = ?", r.ID, r.ProjectID).Delete(&ProjectGithubRepo{})
	return err
}

// GetGithubReposForProject returns the repositories linked to a project, oldest first.
func GetGithubReposForProject(s *xorm.Session, projectID int64) (repos []*ProjectGithubRepo, err error) {
	repos = []*ProjectGithubRepo{}
	err = s.Where("project_id = ?", projectID).OrderBy("id ASC").Find(&repos)
	return
}

// GetProjectsForGithubRepo returns the ids of all projects linked to owner/repo.
func GetProjectsForGithubRepo(s *xorm.Session, owner, repo string) (projectIDs []int64, err error) {
	projectIDs = []int64{}
	err = s.Table("project_github_repos").
		Where("LOWER(owner) = ? AND LOWER(repo) = ?", strings.ToLower(owner), strings.ToLower(repo)).
		Cols("project_id").
		Find(&projectIDs)
	return
}

// CanRead: anyone who can read the project may see which repositories are linked.
func (r *ProjectGithubRepo) CanRead(s *xorm.Session, a web.Auth) (bool, int, error) {
	if _, is := a.(*LinkSharing); is {
		return false, 0, nil
	}
	return (&Project{ID: r.ProjectID}).CanRead(s, a)
}

// CanCreate: linking repositories needs write access to the project.
func (r *ProjectGithubRepo) CanCreate(s *xorm.Session, a web.Auth) (bool, error) {
	return r.canManage(s, a)
}

// CanDelete: unlinking needs write access to the project the link belongs to.
func (r *ProjectGithubRepo) CanDelete(s *xorm.Session, a web.Auth) (bool, error) {
	existing := &ProjectGithubRepo{}
	has, err := s.Where("id = ?", r.ID).Get(existing)
	if err != nil {
		return false, err
	}
	if !has || existing.ProjectID != r.ProjectID {
		return false, nil
	}
	return r.canManage(s, a)
}

func (r *ProjectGithubRepo) canManage(s *xorm.Session, a web.Auth) (bool, error) {
	if _, is := a.(*LinkSharing); is {
		return false, nil
	}
	return (&Project{ID: r.ProjectID}).CanWrite(s, a)
}

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
	"strconv"
	"strings"

	"code.vikunja.io/api/pkg/user"

	"xorm.io/builder"
	"xorm.io/xorm"
)

// GetUserForGithubComments loads the account that posts GitHub deployment comments.
func GetUserForGithubComments(s *xorm.Session, username string) (*user.User, error) {
	return user.GetUserByUsername(s, username)
}

// FindTaskByIdentifierInProjects resolves a task identifier such as "HS-334" within the given
// projects: the project whose identifier is "HS" (case-insensitive) and its task number 334.
// Returns nil (and no error) when nothing matches.
func FindTaskByIdentifierInProjects(s *xorm.Session, key string, projectIDs []int64) (*Task, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	dash := strings.LastIndex(key, "-")
	if dash < 1 || dash == len(key)-1 {
		return nil, nil
	}
	index, convErr := strconv.ParseInt(key[dash+1:], 10, 64)
	if convErr != nil {
		return nil, nil //nolint:nilerr // not a task identifier, so nothing matches
	}
	identifier := strings.ToLower(key[:dash])

	projects := []*Project{}
	err := s.In("id", projectIDs).And("LOWER(identifier) = ?", identifier).Find(&projects)
	if err != nil || len(projects) == 0 {
		return nil, err
	}

	ids := make([]int64, 0, len(projects))
	for _, p := range projects {
		ids = append(ids, p.ID)
	}
	task := &Task{}
	has, err := s.Where(builder.In("project_id", ids)).
		And(builder.Eq{"`index`": index}).
		And(taskNotDeletedCond("tasks")).
		Get(task)
	if err != nil || !has {
		return nil, err
	}
	return task, nil
}

// CanPostGithubComment reports whether the bot account may comment on a task's project.
func CanPostGithubComment(s *xorm.Session, bot *user.User, task *Task) (bool, error) {
	return (&Project{ID: task.ProjectID}).CanWrite(s, bot)
}

// FindTasksByIndexInProjects returns the tasks numbered index in the given projects
// (a bare "#335"/"feat/335-x" reference matches task 335 of each linked project).
func FindTasksByIndexInProjects(s *xorm.Session, index int64, projectIDs []int64) ([]*Task, error) {
	tasks := []*Task{}
	if len(projectIDs) == 0 || index <= 0 {
		return tasks, nil
	}
	err := s.Where(builder.In("project_id", projectIDs)).
		And(builder.Eq{"`index`": index}).
		And(taskNotDeletedCond("tasks")).
		Find(&tasks)
	return tasks, err
}

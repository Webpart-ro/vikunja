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

package migration

import (
	"time"

	"src.techknowlogick.com/xormigrate"
	"xorm.io/xorm"
)

// Webpart: links a project to the GitHub repositories whose branches, pull requests,
// CI runs and deployments show up on its tasks.
type projectGithubRepos20261113130154 struct {
	ID          int64     `xorm:"autoincr not null unique pk"`
	ProjectID   int64     `xorm:"bigint not null index"`
	Owner       string    `xorm:"varchar(100) not null"`
	Repo        string    `xorm:"varchar(100) not null"`
	CreatedByID int64     `xorm:"bigint not null"`
	Created     time.Time `xorm:"created not null"`
}

func (projectGithubRepos20261113130154) TableName() string {
	return "project_github_repos"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20261113130154",
		Description: "add project_github_repos table (Webpart GitHub integration)",
		Migrate: func(tx *xorm.Engine) error {
			return tx.Sync(projectGithubRepos20261113130154{}) //nolint:forbidigo // brand-new table, nothing to drop
		},
		Rollback: func(tx *xorm.Engine) error {
			return tx.DropTables(projectGithubRepos20261113130154{})
		},
	})
}

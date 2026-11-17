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

// Webpart: private links that open one staging website for a client, without a Tasks account.
type stagingClientLinks20261117200000 struct {
	ID          int64     `xorm:"autoincr not null unique pk"`
	Label       string    `xorm:"varchar(250) not null"`
	Host        string    `xorm:"varchar(250) not null index"`
	TokenHash   string    `xorm:"varchar(64) not null unique"`
	ExpiresAt   time.Time `xorm:"not null"`
	LastUsedAt  time.Time `xorm:"null"`
	CreatedByID int64     `xorm:"bigint not null"`
	Created     time.Time `xorm:"created not null"`
}

func (stagingClientLinks20261117200000) TableName() string {
	return "staging_client_links"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20261117200000",
		Description: "add staging_client_links table (Webpart staging client links)",
		Migrate: func(tx *xorm.Engine) error {
			return tx.Sync(stagingClientLinks20261117200000{}) //nolint:forbidigo // brand-new table, nothing to drop
		},
		Rollback: func(tx *xorm.Engine) error {
			return tx.DropTables(stagingClientLinks20261117200000{})
		},
	})
}

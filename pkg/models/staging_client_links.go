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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"xorm.io/xorm"
)

// StagingClientLink is a private link that opens one staging website for a client who has
// no Tasks account (Webpart staging single sign-on). Only the SHA-256 of its key is stored;
// the key itself is shown once, in the link, when it's created.
type StagingClientLink struct {
	ID         int64      `xorm:"autoincr not null unique pk" json:"id" readOnly:"true" doc:"The id of this client link."`
	Label      string     `xorm:"varchar(250) not null" json:"label" doc:"Who the link is for, e.g. the client's name."`
	Host       string     `xorm:"varchar(250) not null index" json:"host" doc:"The one staging host this link opens, e.g. shop-staging.webpart.ro."`
	TokenHash  string     `xorm:"varchar(64) not null unique" json:"-"`
	ExpiresAt  time.Time  `xorm:"not null" json:"expires_at" readOnly:"true" doc:"When the link (and every browser session opened with it) stops working."`
	LastUsedAt *time.Time `xorm:"null" json:"last_used_at" readOnly:"true" doc:"When the link was last opened; null if never."`

	CreatedByID int64     `xorm:"bigint not null" json:"created_by_id" readOnly:"true" doc:"The id of the user who created the link."`
	Created     time.Time `xorm:"created not null" json:"created" readOnly:"true" doc:"When the link was created."`
}

// TableName holds the table name.
func (*StagingClientLink) TableName() string {
	return "staging_client_links"
}

// HashStagingClientKey is how a link key is stored and looked up.
func HashStagingClientKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// CreateStagingClientLink stores a new link and returns it with its key (shown only now).
func CreateStagingClientLink(s *xorm.Session, label, host string, expiresAt time.Time, createdByID int64) (*StagingClientLink, string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	key := base64.RawURLEncoding.EncodeToString(raw)
	link := &StagingClientLink{
		Label:       label,
		Host:        host,
		TokenHash:   HashStagingClientKey(key),
		ExpiresAt:   expiresAt,
		CreatedByID: createdByID,
	}
	if _, err := s.Insert(link); err != nil {
		return nil, "", err
	}
	return link, key, nil
}

// ListStagingClientLinks returns every link, newest first.
func ListStagingClientLinks(s *xorm.Session) ([]*StagingClientLink, error) {
	links := []*StagingClientLink{}
	err := s.OrderBy("id DESC").Find(&links)
	return links, err
}

// DeleteStagingClientLink revokes a link; browsers that opened it lose access within a minute.
// Returns false when there was no such link.
func DeleteStagingClientLink(s *xorm.Session, id int64) (bool, error) {
	n, err := s.Where("id = ?", id).Delete(&StagingClientLink{})
	return n > 0, err
}

// ActiveStagingClientLinkByKey finds the unexpired link for key on host and records the visit.
// Returns nil when the key is unknown, expired or for another host.
func ActiveStagingClientLinkByKey(s *xorm.Session, key, host string, now time.Time) (*StagingClientLink, error) {
	if key == "" {
		return nil, nil
	}
	link := &StagingClientLink{}
	has, err := s.Where("token_hash = ?", HashStagingClientKey(key)).Get(link)
	if err != nil || !has {
		return nil, err
	}
	if link.Host != host || !now.Before(link.ExpiresAt) {
		return nil, nil
	}
	link.LastUsedAt = &now
	if _, err := s.ID(link.ID).Cols("last_used_at").Update(link); err != nil {
		return nil, err
	}
	return link, nil
}

// StagingClientLinkActive reports whether link id still exists, is unexpired and is for host.
func StagingClientLinkActive(s *xorm.Session, id int64, host string, now time.Time) (bool, error) {
	return s.Where("id = ? AND host = ? AND expires_at > ?", id, host, now).Exist(&StagingClientLink{})
}

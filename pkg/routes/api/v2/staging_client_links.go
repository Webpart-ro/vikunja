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

// Webpart staging client links: team members make a private link that opens one staging
// website for a client without a Tasks account (see stagingClientAccess in staging_auth.go).

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/modules/humabridge"
	"code.vikunja.io/api/pkg/modules/stagingauth"
	"code.vikunja.io/api/pkg/user"

	"github.com/danielgtaylor/huma/v2"
)

const (
	stagingClientLinkDefaultDays = 30
	stagingClientLinkMaxDays     = 365
)

type stagingClientLinkListBody struct {
	Body Paginated[*models.StagingClientLink]
}

type stagingClientLinkCreateInput struct {
	Body struct {
		Label string `json:"label" minLength:"1" maxLength:"250" doc:"Who the link is for, e.g. the client's name."`
		Host  string `json:"host" minLength:"1" maxLength:"250" doc:"The staging site the link opens: a host (shop-staging.webpart.ro) or a URL on it."`
		Days  int    `json:"days,omitempty" minimum:"1" maximum:"365" doc:"How many days the link works. Defaults to 30."`
	}
}

type stagingClientLinkCreated struct {
	models.StagingClientLink
	URL string `json:"url" readOnly:"true" doc:"The link to send to the client. It holds the key, which is shown only in this response."`
}

type stagingClientLinkCreateOutput struct {
	Body *stagingClientLinkCreated
}

type stagingClientLinkDeleteInput struct {
	ID int64 `path:"link" doc:"The id of the client link."`
}

// RegisterStagingClientLinkRoutes wires client link management, gated by stagingauth.enabled.
func RegisterStagingClientLinkRoutes(api huma.API) {
	if !stagingauth.Enabled() {
		return
	}
	tags := []string{"staging-auth"}

	Register(api, huma.Operation{
		OperationID: "staging-client-links-list",
		Summary:     "List staging client links",
		Description: "Every client link on this instance, newest first, for all team members (not only the ones you made). Keys are never returned. Interactive logins only: API tokens, link shares and bots get 403.",
		Method:      http.MethodGet, Path: "/staging-auth/client-links", Tags: tags,
	}, stagingClientLinksList)

	Register(api, huma.Operation{
		OperationID: "staging-client-links-create",
		Summary:     "Create a staging client link",
		Description: "Creates a private link that opens one staging site (host only, never the production admin) without a Tasks account, until it expires. The response's url holds the key and is the only time it is shown. Interactive logins only.",
		Method:      http.MethodPost, Path: "/staging-auth/client-links", Tags: tags,
	}, stagingClientLinksCreate)

	Register(api, huma.Operation{
		OperationID: "staging-client-links-delete",
		Summary:     "Revoke a staging client link",
		Description: "Deletes the link. Browsers that already opened it lose access within a minute. Interactive logins only.",
		Method:      http.MethodDelete, Path: "/staging-auth/client-links/{link}", Tags: tags,
	}, stagingClientLinksDelete)
}

func init() { AddRouteRegistrar(RegisterStagingClientLinkRoutes) }

// stagingTeamUser returns the caller if it is an active, interactive (non-bot, non-token) user.
func stagingTeamUser(ctx context.Context) (*user.User, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	u, ok := a.(*user.User)
	ec := humabridge.EchoContextFrom(ctx)
	if !ok || ec == nil || ec.Get("api_token") != nil {
		return nil, huma.Error403Forbidden("staging client links need an interactive login")
	}
	s := db.NewSession()
	defer s.Close()
	fresh, err := user.GetUserByID(s, u.ID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	if fresh.IsBot() || fresh.Status != user.StatusActive {
		return nil, huma.Error403Forbidden("staging client links need an interactive login")
	}
	return fresh, nil
}

func stagingClientLinksList(ctx context.Context, _ *struct{}) (*stagingClientLinkListBody, error) {
	if _, err := stagingTeamUser(ctx); err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer s.Close()
	links, err := models.ListStagingClientLinks(s)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &stagingClientLinkListBody{Body: NewPaginated(links, int64(len(links)), 1, len(links))}, nil
}

func stagingClientLinksCreate(ctx context.Context, in *stagingClientLinkCreateInput) (*stagingClientLinkCreateOutput, error) {
	u, err := stagingTeamUser(ctx)
	if err != nil {
		return nil, err
	}
	host, err := stagingauth.NormalizeClientHost(in.Body.Host)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	days := in.Body.Days
	if days == 0 {
		days = stagingClientLinkDefaultDays
	}
	if days < 1 || days > stagingClientLinkMaxDays {
		return nil, huma.Error400BadRequest(fmt.Sprintf("days must be between 1 and %d", stagingClientLinkMaxDays))
	}

	s := db.NewSession()
	defer s.Close()
	expires := time.Now().Add(time.Duration(days) * 24 * time.Hour).Truncate(time.Second)
	link, key, err := models.CreateStagingClientLink(s, in.Body.Label, host, expires, u.ID)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		return nil, translateDomainError(err)
	}
	return &stagingClientLinkCreateOutput{Body: &stagingClientLinkCreated{
		StagingClientLink: *link,
		URL:               stagingauth.ClientLinkURL(host, key),
	}}, nil
}

func stagingClientLinksDelete(ctx context.Context, in *stagingClientLinkDeleteInput) (*emptyBody, error) {
	if _, err := stagingTeamUser(ctx); err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer s.Close()
	found, err := models.DeleteStagingClientLink(s, in.ID)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		return nil, translateDomainError(err)
	}
	if !found {
		return nil, huma.Error404NotFound("client link not found")
	}
	forgetStagingClientLink(in.ID)
	return &emptyBody{}, nil
}

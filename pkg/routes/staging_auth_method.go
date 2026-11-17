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

package routes

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// stagingAuthVerifyPath is the Traefik forwardAuth endpoint (pkg/routes/api/v2/staging_auth.go).
const stagingAuthVerifyPath = "/api/v2/staging-auth/verify"

// Webpart: Traefik's forwardAuth calls the verify endpoint with the original request's
// method and headers (Content-Type included), but never its body. Without this, a POST to
// a staging site (add to cart, back-office login) hit the GET-only route and the form-body
// guard, and Traefik passed the 405/415 straight to the browser. Every call is turned into
// a bodiless GET before routing; the original method stays in X-Forwarded-Method.
func normalizeStagingAuthVerify() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			if req.URL.Path == stagingAuthVerifyPath && req.Method != http.MethodGet {
				if req.Header.Get("X-Forwarded-Method") == "" {
					req.Header.Set("X-Forwarded-Method", req.Method)
				}
				req.Method = http.MethodGet
				req.Header.Del(echo.HeaderContentType)
				req.Header.Del("Content-Length")
				req.ContentLength = 0
				req.Body = http.NoBody
			}
			return next(c)
		}
	}
}

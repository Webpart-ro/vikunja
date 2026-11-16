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

package stagingauth

// Link previews: chat apps (Discord, Slack, WhatsApp, …) fetch a shared staging URL without
// a session. Instead of following the redirect to the Vikunja login page (and showing
// "Webpart Tasks / Vikunja …" in the embed), they get a small Webpart-branded card. They are
// never let through to the staging site itself; the User-Agent can be spoofed.

import (
	"html"
	"net/url"
	"strings"

	"code.vikunja.io/api/pkg/config"
)

// previewBots are substrings of link-unfurler User-Agents (case-insensitive).
var previewBots = []string{
	"discordbot",
	"slackbot",
	"slack-imgproxy",
	"facebookexternalhit",
	"facebot",
	"twitterbot",
	"whatsapp",
	"telegrambot",
	"linkedinbot",
	"skypeuripreview",
	"microsoftpreview",
	"pinterest",
	"redditbot",
	"mattermost",
	"embedly",
	"iframely",
	"vkshare",
	"viber",
	"signal",
}

// IsLinkPreviewBot reports whether the User-Agent belongs to a chat/social link unfurler.
func IsLinkPreviewBot(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	if ua == "" {
		return false
	}
	for _, bot := range previewBots {
		if strings.Contains(ua, bot) {
			return true
		}
	}
	return false
}

// PreviewURL is the public page that renders the Webpart card for a staging URL.
func PreviewURL(target string) string {
	return publicBase() + "/api/v2/staging-auth/preview?rd=" + url.QueryEscape(target)
}

// PreviewImage is the og:image used for staging link previews.
const PreviewImage = "https://webpart.ro/og.png"

// PreviewHTML renders the Open Graph card for a validated staging URL.
func PreviewHTML(target *url.URL) string {
	host := target.Hostname()
	title := "Webpart · staging · " + host
	description := "Preview build by Webpart. Sign in with Webpart Tasks to open it."
	t := html.EscapeString(title)
	d := html.EscapeString(description)
	u := html.EscapeString(target.String())
	img := html.EscapeString(PreviewImage)

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"robots\" content=\"noindex, nofollow\">\n")
	b.WriteString("<title>" + t + "</title>\n")
	b.WriteString("<meta name=\"description\" content=\"" + d + "\">\n")
	b.WriteString("<meta name=\"theme-color\" content=\"#10312b\">\n")
	b.WriteString("<meta property=\"og:site_name\" content=\"Webpart\">\n")
	b.WriteString("<meta property=\"og:type\" content=\"website\">\n")
	b.WriteString("<meta property=\"og:title\" content=\"" + t + "\">\n")
	b.WriteString("<meta property=\"og:description\" content=\"" + d + "\">\n")
	b.WriteString("<meta property=\"og:url\" content=\"" + u + "\">\n")
	b.WriteString("<meta property=\"og:image\" content=\"" + img + "\">\n")
	b.WriteString("<meta property=\"og:image:width\" content=\"1200\">\n")
	b.WriteString("<meta property=\"og:image:height\" content=\"630\">\n")
	b.WriteString("<meta name=\"twitter:card\" content=\"summary_large_image\">\n")
	b.WriteString("<meta name=\"twitter:title\" content=\"" + t + "\">\n")
	b.WriteString("<meta name=\"twitter:description\" content=\"" + d + "\">\n")
	b.WriteString("<meta name=\"twitter:image\" content=\"" + img + "\">\n")
	b.WriteString("</head><body>\n")
	b.WriteString("<p>" + d + "</p>\n")
	b.WriteString("<p><a href=\"" + html.EscapeString(LoginURL(target.String())) + "\">Open " + html.EscapeString(host) + "</a></p>\n")
	b.WriteString("</body></html>\n")
	return b.String()
}

func publicBase() string {
	return strings.TrimRight(config.ServicePublicURL.GetString(), "/")
}

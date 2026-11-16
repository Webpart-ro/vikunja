# tasks.webpart.ro auto-deploy (homeserver)

Merging a PR into `webpart` builds `ghcr.io/webpart-ro/vikunja:<short sha>` and `:latest`
(`.github/workflows/webpart-image.yml`). On homeserver, `vikunja-autodeploy.timer` checks
every minute. When `:latest` carries a new commit, it pins `/srv/apps/vikunja/compose.yml` to
it, restarts the app, checks `/api/v1/info`, and rolls back to `compose.yml.previous` if that
fails. A SHA that failed is recorded in `/srv/apps/vikunja/.autodeploy-failed` and skipped
until the next merge.

The server pulls; GitHub never gets access to it (the fork is public, so it doesn't use the
self-hosted runner).

## Install / update (as root, from a checkout of a merged `webpart`)

```sh
install -m 755 deploy/homeserver/vikunja-autodeploy.sh /usr/local/bin/vikunja-autodeploy.sh
install -m 644 deploy/homeserver/vikunja-autodeploy.service deploy/homeserver/vikunja-autodeploy.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now vikunja-autodeploy.timer
```

## Operate

```sh
journalctl -u vikunja-autodeploy -n 50     # what it did
systemctl start vikunja-autodeploy          # deploy now instead of waiting
systemctl disable --now vikunja-autodeploy.timer   # pause auto-deploys
```

Manual rollback: `cp compose.yml.previous compose.yml && docker compose up -d vikunja` in
`/srv/apps/vikunja`. Startup migrations are not reverted by a rollback.

## CI

PR checks are lint, typecheck, frontend unit tests and builds (~3 min). The full upstream
suite (every database, e2e) runs on demand: Actions → ci → Run workflow (`full` checked).

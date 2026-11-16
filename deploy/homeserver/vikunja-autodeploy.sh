#!/usr/bin/env bash
# Pull-based deploy for tasks.webpart.ro (homeserver, /srv/apps/vikunja).
#
# ghcr.io/webpart-ro/vikunja:latest is only pushed by .github/workflows/webpart-image.yml
# on pushes to the `webpart` branch, i.e. merged PRs. This script, run by
# vikunja-autodeploy.timer, pins compose.yml to that image's short commit SHA, restarts
# the app, checks /api/v1/info, and rolls back on failure.
#
# Note: Vikunja runs DB migrations on startup. A rollback restores the old image but does
# not undo migrations a failed release already applied.
set -euo pipefail

DIR="${VIKUNJA_DIR:-/srv/apps/vikunja}"
IMAGE="${VIKUNJA_IMAGE:-ghcr.io/webpart-ro/vikunja}"
SERVICE="${VIKUNJA_SERVICE:-vikunja}"
COMPOSE="$DIR/compose.yml"
FAILED="$DIR/.autodeploy-failed"

exec 9>/run/vikunja-autodeploy.lock
flock -n 9 || exit 0

log() { echo "vikunja-autodeploy: $*"; }

current="$(grep -oE "image: ${IMAGE}:[0-9a-f]{7,40}" "$COMPOSE" | grep -oE '[0-9a-f]{7,40}$' || true)"
if [ -z "$current" ]; then
  log "no pinned ${IMAGE}:<sha> in $COMPOSE, refusing to touch it"
  exit 1
fi

docker pull -q "${IMAGE}:latest" >/dev/null
revision="$(docker image inspect "${IMAGE}:latest" \
  --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}')"
if ! [[ "$revision" =~ ^[0-9a-f]{40}$ ]]; then
  log "${IMAGE}:latest has no revision label (got '$revision')"
  exit 1
fi
latest="${revision:0:${#current}}" # same length as the pinned tag (7 = metadata-action short sha)

[ "$latest" = "$current" ] && exit 0

if [ -f "$FAILED" ] && grep -qx "$latest" "$FAILED"; then
  exit 0 # already failed once; wait for the next merge
fi

log "deploying $current -> $latest"
docker tag "${IMAGE}:latest" "${IMAGE}:${latest}"
cp "$COMPOSE" "$COMPOSE.previous"
sed -i "s#image: ${IMAGE}:${current}#image: ${IMAGE}:${latest}#" "$COMPOSE"

# The image is FROM scratch (no shell/curl), so check it from the host via the container IP.
healthy() {
  for _ in $(seq 1 60); do
    cid="$(docker compose -f "$COMPOSE" ps -q "$SERVICE" 2>/dev/null || true)"
    if [ -n "$cid" ]; then
      for ip in $(docker inspect "$cid" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}'); do
        if curl -fsS -o /dev/null --max-time 3 "http://${ip}:3456/api/v1/info"; then
          return 0
        fi
      done
    fi
    sleep 3
  done
  return 1
}

if docker compose -f "$COMPOSE" up -d "$SERVICE" && healthy; then
  log "deployed $latest"
  docker images "$IMAGE" --format '{{.Tag}}' \
    | grep -E '^[0-9a-f]{7,40}$' \
    | grep -vx -e "$latest" -e "$current" \
    | xargs -r -I{} docker rmi "${IMAGE}:{}" >/dev/null 2>&1 || true
  exit 0
fi

log "health check failed for $latest, rolling back to $current"
echo "$latest" >> "$FAILED"
cp "$COMPOSE.previous" "$COMPOSE"
docker compose -f "$COMPOSE" up -d "$SERVICE" || true
exit 1

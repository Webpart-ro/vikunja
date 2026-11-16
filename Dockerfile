# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
FROM --platform=$BUILDPLATFORM node:24.21.0-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS frontendbuilder

WORKDIR /build

ENV PNPM_CACHE_FOLDER=.cache/pnpm/
ENV PUPPETEER_SKIP_DOWNLOAD=true
ENV CYPRESS_INSTALL_BINARY=0

COPY frontend/pnpm-lock.yaml frontend/package.json frontend/pnpm-workspace.yaml ./
RUN npm install -g corepack && corepack enable && \
    pnpm install --frozen-lockfile
COPY frontend/ ./
ARG RELEASE_VERSION=dev
RUN echo "{\"VERSION\": \"${RELEASE_VERSION/-g/-}\"}" > src/version.json && pnpm run build

# Webpart: plain Go instead of the multi-GB xgo cross-compile image (we only ship
# linux/amd64, built on an amd64 runner). Static binary like upstream's xgo build;
# CGO stays on for the sqlite driver.
FROM golang:1.27-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS apibuilder

WORKDIR /go/src/code.vikunja.io/api
COPY go.mod go.sum ./
# Modules live in a layer (not a cache mount) so the GitHub Actions layer cache keeps them.
RUN go mod download
COPY . ./
COPY --from=frontendbuilder /build/dist ./frontend/dist

ARG RELEASE_VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
	mkdir -p /build && \
	CGO_ENABLED=1 go build -trimpath -tags "osusergo netgo" \
	-ldflags "-s -w -linkmode external -extldflags '-static' -X 'code.vikunja.io/api/pkg/version.Version=${RELEASE_VERSION}' -X 'main.Tags=osusergo netgo'" \
	-o /build/vikunja-webpart .
RUN mkdir -p /tmp && chmod 1777 /tmp

#  ┬─┐┬ ┐┌┐┐┌┐┐┬─┐┬─┐
#  │┬┘│ │││││││├─ │┬┘
#  ┘└┘┘─┘┘└┘┘└┘┴─┘┘└┘

# The actual image
FROM scratch

LABEL org.opencontainers.image.authors='maintainers@vikunja.io'
LABEL org.opencontainers.image.url='https://vikunja.io'
LABEL org.opencontainers.image.documentation='https://vikunja.io/docs'
LABEL org.opencontainers.image.source='https://github.com/Webpart-ro/vikunja'
LABEL org.opencontainers.image.licenses='AGPLv3'
LABEL org.opencontainers.image.title='Webpart Tasks (Vikunja fork)'

WORKDIR /app/vikunja
ENTRYPOINT [ "/app/vikunja/vikunja" ]
EXPOSE 3456

COPY --from=apibuilder --chown=1000:1000 --chmod=1777 /tmp /tmp

USER 1000

ENV VIKUNJA_SERVICE_ROOTPATH=/app/vikunja/
ENV VIKUNJA_DATABASE_PATH=/db/vikunja.db
# Mitigates CVE-2026-98432 (kernel IPv4 TCP use-after-free) by disabling MPTCP
# on Vikunja's listen sockets. Remove once host kernels are patched.
ENV GODEBUG=multipathtcp=0

COPY --from=apibuilder /build/vikunja-* vikunja
COPY --from=apibuilder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

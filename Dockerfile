ARG BUILDKIT_BASE_IMAGE=moby/buildkit:v0.33.0-rootless@sha256:80b15f0735e87bab7bf59ec4d695dfb4a7cfb25521cf56dc75d6f256285b63ef

FROM ${BUILDKIT_BASE_IMAGE} AS buildkit-client

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT_SHA=unknown
ARG BUILD_TIME=unknown
ENV BUILD_LDFLAGS="-s -w -X github.com/Stealth-deplover/stealth/internal/buildinfo.Version=${VERSION} -X github.com/Stealth-deplover/stealth/internal/buildinfo.Commit=${COMMIT_SHA} -X github.com/Stealth-deplover/stealth/internal/buildinfo.BuildTime=${BUILD_TIME}"
RUN CGO_ENABLED=0 go build -trimpath -ldflags="${BUILD_LDFLAGS}" -o /out/stealth-api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="${BUILD_LDFLAGS}" -o /out/stealth-worker ./cmd/worker
RUN CGO_ENABLED=0 go build -trimpath -ldflags="${BUILD_LDFLAGS}" -o /out/stealth-cloudflare-import-init ./cmd/cloudflare-import-init
RUN CGO_ENABLED=0 go build -trimpath -ldflags="${BUILD_LDFLAGS}" -o /out/stealth-cloudflare-state-init ./cmd/cloudflare-state-init
RUN CGO_ENABLED=0 go build -trimpath -ldflags="${BUILD_LDFLAGS}" -o /out/stealth-ingress-control ./cmd/ingress-control
RUN CGO_ENABLED=0 go build -trimpath -ldflags="${BUILD_LDFLAGS}" -o /out/stealth-migrate ./cmd/migrate

FROM alpine:3.24 AS runtime-base
ARG VERSION=dev
ARG COMMIT_SHA=unknown
ARG BUILD_TIME=unknown
# Keep the application identity stable because the host installer prepares
# bind-mounted generated state for this numeric owner. Do not make these build
# arguments: changing them would silently invalidate the host ownership
# contract.
RUN apk add --no-cache ca-certificates wget && addgroup -S -g 10001 stealth && adduser -S -D -u 10001 -G stealth stealth
RUN mkdir -p /var/lib/stealth/storage /var/lib/stealth/runner-staging /var/lib/stealth/app-build-staging && chown -R stealth:stealth /var/lib/stealth
VOLUME ["/var/lib/stealth/storage", "/var/lib/stealth/runner-staging"]
WORKDIR /app
LABEL org.opencontainers.image.title="Stealth" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT_SHA" \
      org.opencontainers.image.created="$BUILD_TIME" \
      org.opencontainers.image.source="https://github.com/Stealth-deplover/stealth"

FROM runtime-base AS api
COPY --from=build /out/stealth-api /usr/local/bin/stealth-api
USER stealth
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/stealth-api"]

# The setup image is intentionally a separate target. It serves the temporary
# browser wizard and reads/writes only the shared encrypted setup state; the
# host CLI owns production installation and Docker execution. It stays root in
# the container so a host CLI with a different UID can use the state bind, but
# it has no Docker client or daemon socket.
FROM runtime-base AS setup
COPY --from=build /out/stealth-api /usr/local/bin/stealth-api
USER root
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/stealth-api"]

FROM runtime-base AS worker
RUN apk add --no-cache docker-cli
COPY --from=build /out/stealth-worker /usr/local/bin/stealth-worker
COPY --from=build /out/stealth-cloudflare-import-init /usr/local/bin/stealth-cloudflare-import-init
COPY --from=build /out/stealth-cloudflare-state-init /usr/local/bin/stealth-cloudflare-state-init
COPY --from=buildkit-client /usr/bin/buildctl /usr/local/bin/buildctl
USER stealth
EXPOSE 9091
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:9091/healthz >/dev/null || exit 1
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/stealth-worker"]

# The host invokes this one-shot image through Compose. It deliberately does
# not inherit runtime-base's storage/staging volumes or install Docker tools.
FROM alpine:3.24 AS ingress-control
ARG VERSION=dev
ARG COMMIT_SHA=unknown
ARG BUILD_TIME=unknown
RUN apk add --no-cache ca-certificates && addgroup -S -g 10001 stealth && adduser -S -D -u 10001 -G stealth stealth
WORKDIR /app
COPY --from=build /out/stealth-ingress-control /usr/local/bin/stealth-ingress-control
USER stealth
LABEL org.opencontainers.image.title="Stealth ingress control" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT_SHA" \
      org.opencontainers.image.created="$BUILD_TIME" \
      org.opencontainers.image.source="https://github.com/Stealth-deplover/stealth"
ENTRYPOINT ["/usr/local/bin/stealth-ingress-control"]

FROM runtime-base AS migrate
COPY --from=build /out/stealth-migrate /usr/local/bin/stealth-migrate
USER stealth
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/stealth-migrate"]

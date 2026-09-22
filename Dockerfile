# syntax=docker/dockerfile:1
#
# Three stages: bun builds the SolidJS bundle into web/static/dist, Go embeds
# it (web/efs.go, !dev build tag) into a static binary, and the result runs on
# distroless. The same file is built by .gitea/workflows/image.yaml and
# .github/workflows/image.yml, so both registries carry identical images.
#
# The runtime stage is distroless static today. PDF statement import (roadmap
# P4, docs/ARCHITECTURE.md) needs poppler's pdftotext, and at that point this
# stage becomes debian:bookworm-slim with poppler-utils.

FROM oven/bun:1.3.14 AS web
WORKDIR /src/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build:prod

FROM golang:1.26.8-bookworm AS build
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/static/dist ./web/static/dist
ENV CGO_ENABLED=0
RUN go build -tags prod -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/rigel-ledger ./cmd/rigel-ledger \
 && go build -tags prod -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/rigel-ledger-cli ./cmd/cli

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/rigel-ledger /out/rigel-ledger-cli /usr/local/bin/
ENV APP_NAME=rigel-ledger \
    APP_ENV=production \
    APP_PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/rigel-ledger"]

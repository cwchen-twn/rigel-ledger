# syntax=docker/dockerfile:1
#
# rigel-ledger-ci: the image the `ci` job runs in, so a run installs nothing.
# Before it, every run spent 88 s on mise's tools, 23 s apt-installing
# PostgreSQL and ~18 s on pre-commit's hook environments (run 1325, #147);
# the rigel-sync job spent 39 s setting up Chrome (run 1361, #149).
#
# It holds tools only, no code and no data: the build context is mise.toml
# and .pre-commit-config.yaml (ci/Dockerfile.dockerignore). Built by
# .gitea/workflows/ci-image.yaml and .github/workflows/ci-image.yml whenever
# one of those changes on main, or locally with `make ci/image`. A job still
# runs `mise install` first: a no-op when the image matches mise.toml, and
# just the difference when a PR bumps a pin before the image is rebuilt.
#
# Go's build and module caches are NOT here: they change with every commit.
# The runner mounts them from a volume (catopia/hcloud#202).

FROM debian:bookworm

# The CI basics: git for checkout and gitleaks, make, gcc + libc for
# `go test -race` (cgo), python3 for nothing but pre-commit's own shims.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl git gnupg make gcc libc6-dev xz-utils unzip python3 \
 && rm -rf /var/lib/apt/lists/*

# The rigel-sync job's browser: Debian's chromium (and the fonts its tests
# render), as integrations/rigel-sync/Dockerfile installs it, plus the CAPTCHA
# model at its pinned commit and sha256 (src/ocr/captcha.ts), so a run neither
# sets up Chrome nor downloads the model. openssl, which the pretend banks use
# for their certificates, comes with ca-certificates.
RUN apt-get update \
 && apt-get install -y --no-install-recommends chromium fonts-liberation \
 && rm -rf /var/lib/apt/lists/* \
 && chromium --version \
 && mkdir -p -m 755 /opt/rigel-sync/models
ADD --chmod=644 --checksum=sha256:b8f2ad9cbc1f2e3922a6cb9459e30824e7e2467f3fb4fd61420640e34ea0bf68 \
    https://raw.githubusercontent.com/sml2h3/ddddocr/f0ee6b122e9f6b4ccc0cd96032f93cd43721543f/ddddocr/common_old.onnx \
    /opt/rigel-sync/models/ddddocr-common_old.onnx
ADD --chmod=644 --checksum=sha256:8f9defe3bfe64316168989830d40793a5074f25a8b13f3bb8b093914a7c27f73 \
    https://raw.githubusercontent.com/sml2h3/ddddocr/f0ee6b122e9f6b4ccc0cd96032f93cd43721543f/LICENSE \
    /opt/rigel-sync/models/ddddocr-LICENSE
# CI runs as root, which src/browser/cloudflare.ts already treats as
# no-sandbox; RIGEL_SYNC_NO_SANDBOX says so for any other user too.
ENV CHROME_PATH=/usr/bin/chromium \
    RIGEL_SYNC_NO_SANDBOX=1 \
    RIGEL_SYNC_OCR_MODEL=/opt/rigel-sync/models/ddddocr-common_old.onnx

# PostgreSQL 18, the major hcloud runs, from PGDG: the same packages the CI
# job used to install on every run. The job starts it (pg_ctlcluster).
RUN install -d /usr/share/postgresql-common/pgdg \
 && curl -fsSL --retry 5 --retry-all-errors \
      -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
      https://www.postgresql.org/media/keys/ACCC4CF8.asc \
 && echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" \
      > /etc/apt/sources.list.d/pgdg.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends postgresql-18 \
 && rm -rf /var/lib/apt/lists/*

# mise at the version the workflows pin (MISE_VERSION, passed by them and by
# `make ci/image`), checked against the release's SHASUMS256.txt.
ARG MISE_VERSION
RUN test -n "$MISE_VERSION" \
 && cd /tmp \
 && curl -fsSLO --retry 5 --retry-all-errors "https://github.com/jdx/mise/releases/download/v${MISE_VERSION}/mise-v${MISE_VERSION}-linux-x64" \
 && curl -fsSL --retry 5 --retry-all-errors "https://github.com/jdx/mise/releases/download/v${MISE_VERSION}/SHASUMS256.txt" \
      | grep " ./mise-v${MISE_VERSION}-linux-x64\$" | sed 's| ./| |' | sha256sum -c - \
 && install -m 0755 "mise-v${MISE_VERSION}-linux-x64" /usr/local/bin/mise \
 && rm -f "mise-v${MISE_VERSION}-linux-x64"

# Tools land outside any home directory, and the shims come first on PATH so
# every job resolves the repo's mise.toml. The job's checkout path is not
# known here, so every config is trusted: this image only ever runs CI.
ENV MISE_DATA_DIR=/opt/mise \
    MISE_CACHE_DIR=/opt/mise/cache \
    MISE_TRUSTED_CONFIG_PATHS=/ \
    MISE_YES=1 \
    PRE_COMMIT_HOME=/opt/pre-commit \
    PATH=/opt/mise/shims:$PATH

WORKDIR /opt/ci
COPY mise.toml .pre-commit-config.yaml ./
# GH_PUBLIC_TOKEN (a no-scope github.com PAT) lifts api.github.com's anonymous
# 60/h limit for the ~15 installs. A build secret: never in a layer.
RUN --mount=type=secret,id=gh_token,env=GITHUB_TOKEN \
    mise install \
 && ln -s "$(mise which node)" /usr/local/bin/node \
 && node --version \
 && rm -rf /opt/mise/cache
# JavaScript actions (checkout) run `node` from PATH in the job container,
# outside any directory with a mise.toml, where the shim would find no
# version: hence the real symlink above.

# pre-commit's hook environments (pre-commit-hooks, check-jsonschema,
# gitleaks), which `make lint/hygiene` otherwise builds on every run.
# install-hooks wants a git repository.
RUN --mount=type=secret,id=gh_token,env=GITHUB_TOKEN \
    git init -q . \
 && pre-commit install-hooks \
 && rm -rf .git

WORKDIR /

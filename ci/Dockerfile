# syntax=docker/dockerfile:1
#
# rigel-ledger-ci: the image the `ci` job runs in, so a run installs nothing.
# Before it, every run spent 88 s on mise's tools, 23 s apt-installing
# PostgreSQL and ~18 s on pre-commit's hook environments (run 1325, #147).
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

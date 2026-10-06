# syntax=docker/dockerfile:1
#
# rigel-ledger-sync: the rigel-sync runner with Node and Chromium inside, so a
# person installs Docker and nothing else (docs/ARCHITECTURE.md, "Packaged
# as a container"). Built from integrations/rigel-sync as the context by
# .gitea/workflows/image.yaml and .github/workflows/image.yml, linux/amd64.
#
#   docker run -d -e RIGEL_URL=... -e RUNNER_TOKEN=... -v rigel-sync:/data IMAGE
#   docker run --rm -it -v rigel-sync:/data IMAGE try tw-cathaybk
#
# Node runs src/ directly (type stripping), so there is no build: the first
# stage only installs the production dependencies and drops what Node never
# loads (onnxruntime-web's browser builds, 130 MB of its 139).

FROM oven/bun:1.4.2 AS deps
WORKDIR /app
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile --production \
 && find node_modules/onnxruntime-web/dist -type f \
      ! -name 'ort.node.min.*' ! -name 'ort-wasm-simd-threaded.mjs' \
      ! -name 'ort-wasm-simd-threaded.wasm' ! -name package.json -delete \
 && find node_modules -name '*.map' -delete

FROM node:22.23.3-bookworm-slim
# Debian's chromium (security-updated in bookworm-security); tini reaps the
# renderer processes Chrome leaves when it is killed, which Node as PID 1
# would not.
RUN apt-get update \
 && apt-get install -y --no-install-recommends chromium fonts-liberation tini ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir /data && chown node:node /data \
 && mkdir -p -m 755 /opt/rigel-sync/models
# The CAPTCHA model (src/ocr/captcha.ts), at its pinned commit and sha256,
# with its licence, so a runner never downloads it.
ADD --chmod=644 --checksum=sha256:b8f2ad9cbc1f2e3922a6cb9459e30824e7e2467f3fb4fd61420640e34ea0bf68 \
    https://raw.githubusercontent.com/sml2h3/ddddocr/f0ee6b122e9f6b4ccc0cd96032f93cd43721543f/ddddocr/common_old.onnx \
    /opt/rigel-sync/models/ddddocr-common_old.onnx
ADD --chmod=644 --checksum=sha256:8f9defe3bfe64316168989830d40793a5074f25a8b13f3bb8b093914a7c27f73 \
    https://raw.githubusercontent.com/sml2h3/ddddocr/f0ee6b122e9f6b4ccc0cd96032f93cd43721543f/LICENSE \
    /opt/rigel-sync/models/ddddocr-LICENSE

WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY package.json ./
COPY src ./src
COPY vendor/all-set-tw ./vendor/all-set-tw

# Chrome's sandbox needs user namespaces, which Docker's and Kubernetes'
# default seccomp profiles do not give a container; the container is the
# boundary instead (RIGEL_SYNC_NO_SANDBOX, src/browser/cloudflare.ts).
ENV NODE_ENV=production \
    DATA_DIR=/data \
    CHROME_PATH=/usr/bin/chromium \
    RIGEL_SYNC_NO_SANDBOX=1 \
    RIGEL_SYNC_OCR_MODEL=/opt/rigel-sync/models/ddddocr-common_old.onnx
VOLUME /data
USER node
ENTRYPOINT ["/usr/bin/tini", "--", "node", "--disable-warning=ExperimentalWarning", "src/main.ts"]
CMD ["run"]

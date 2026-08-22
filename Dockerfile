ARG BUILDPLATFORM
ARG TEMPL_VERSION=v0.3.1020

FROM --platform=$BUILDPLATFORM ghcr.io/a-h/templ:${TEMPL_VERSION} AS templ

FROM --platform=$BUILDPLATFORM golang:1.27.0 AS builder

# checkov:skip=CKV_DOCKER_2:HEALTHCHECK - managed by App Runner
# checkov:skip=CKV_DOCKER_3:nonroot distroless runtime user defined below

WORKDIR /src

ARG TAILWIND_VERSION=v4.2.4
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_REVISION=development

# Preserve the standard trust store before adding build-only trust. The final
# image is assembled from this copy plus the distinct runtime secret below.
RUN cp /etc/ssl/certs/ca-certificates.crt /tmp/runtime-ca-certificates.crt

# BuildKit keeps the host's managed proxy CA out of image history. `task compose`
# supplies this bundle when Docker traffic is intercepted by a trusted proxy.
ARG BUILD_CA_BUNDLE_DIGEST=empty
RUN --mount=type=secret,id=build_ca_bundle,required=false \
  printf '%s' "${BUILD_CA_BUNDLE_DIGEST}" >/dev/null; \
  if [ -s /run/secrets/build_ca_bundle ]; then \
  cat /run/secrets/build_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; \
  fi

RUN os="$(uname -s)" \
  && arch="$(uname -m)" \
  && case "$os/$arch" in \
  Linux/x86_64|Linux/amd64) asset="tailwindcss-linux-x64" ;; \
  Linux/aarch64|Linux/arm64) asset="tailwindcss-linux-arm64" ;; \
  *) echo "Unsupported platform for Tailwind CLI: $os/$arch" && exit 1 ;; \
  esac \
  && curl -fsSL -o /usr/local/bin/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/${asset}" \
  && chmod +x /usr/local/bin/tailwindcss

COPY --from=templ /ko-app/templ /usr/local/bin/templ

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate templ components with the pinned official generator image so the
# build does not need a second Go module download through the Docker proxy.
RUN tailwindcss -i ./cmd/web/tailwind/app.css -o ./cmd/web/static/css/tailwind.css --minify \
  && templ generate \
  && CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
  go build -trimpath \
  -ldflags="-s -w -X portfolio/internal/buildinfo.revision=${BUILD_REVISION}" \
  -o /out/portfolio-server ./cmd/server

ARG RUNTIME_CA_BUNDLE_DIGEST=empty
RUN --mount=type=secret,id=runtime_ca_bundle,required=false \
  printf '%s' "${RUNTIME_CA_BUNDLE_DIGEST}" >/dev/null; \
  cp /tmp/runtime-ca-certificates.crt /out/ca-certificates.crt; \
  if [ -s /run/secrets/runtime_ca_bundle ]; then \
  cat /run/secrets/runtime_ca_bundle >> /out/ca-certificates.crt; \
  fi

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Keep the standard trust store plus only the distinct runtime CA bundle. The
# build-only CA never crosses the builder-stage boundary.
COPY --from=builder /out/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/portfolio-server /app/portfolio-server
COPY --from=builder /src/cmd/web/static /app/cmd/web/static

EXPOSE 8080

ENTRYPOINT ["/app/portfolio-server"]

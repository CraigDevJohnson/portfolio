ARG BUILDPLATFORM

FROM --platform=$BUILDPLATFORM golang:1.26.4 AS builder

WORKDIR /src

ARG TAILWIND_VERSION=v4.2.4
ARG TARGETOS=linux
ARG TARGETARCH=arm64

RUN apt-get update \
  && apt-get install -y --no-install-recommends curl ca-certificates \
  && rm -rf /var/lib/apt/lists/*

RUN os="$(uname -s)" \
  && arch="$(uname -m)" \
  && case "$os/$arch" in \
  Linux/x86_64|Linux/amd64) asset="tailwindcss-linux-x64" ;; \
  Linux/aarch64|Linux/arm64) asset="tailwindcss-linux-arm64" ;; \
  *) echo "Unsupported platform for Tailwind CLI: $os/$arch" && exit 1 ;; \
  esac \
  && curl -fsSL -o /usr/local/bin/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/${asset}" \
  && chmod +x /usr/local/bin/tailwindcss

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate templ components using the templ module version selected in go.mod.
RUN templ_version="$(go list -m -f '{{.Version}}' github.com/a-h/templ)" \
  && tailwindcss -i ./cmd/web/tailwind/app.css -o ./cmd/web/static/css/tailwind.css --minify \
  && go run "github.com/a-h/templ/cmd/templ@${templ_version}" generate \
  && CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -trimpath -ldflags='-s -w' -o /out/portfolio-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/portfolio-server /app/portfolio-server
COPY --from=builder /src/cmd/web/static /app/cmd/web/static

EXPOSE 8080

ENTRYPOINT ["/app/portfolio-server"]

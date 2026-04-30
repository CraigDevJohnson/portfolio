FROM golang:1.26.1-bookworm AS builder

WORKDIR /src

ARG TAILWIND_VERSION=v4.2.4

RUN apt-get update \
  && apt-get install -y --no-install-recommends curl ca-certificates \
  && rm -rf /var/lib/apt/lists/*

RUN arch="$(dpkg --print-architecture)" \
  && case "$arch" in \
  amd64) asset="tailwindcss-linux-x64" ;; \
  arm64) asset="tailwindcss-linux-arm64" ;; \
  *) echo "Unsupported Debian architecture for Tailwind CLI: $arch" && exit 1 ;; \
  esac \
  && curl -fsSL -o /usr/local/bin/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/${asset}" \
  && chmod +x /usr/local/bin/tailwindcss

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate templ components using the version pinned in go.mod
RUN tailwindcss -i ./cmd/web/tailwind/app.css -o ./cmd/web/static/css/tailwind.css --minify \
  && go tool templ generate \
  && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/portfolio-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/portfolio-server /app/portfolio-server
COPY --from=builder /src/cmd/web/static /app/cmd/web/static

EXPOSE 8080

ENTRYPOINT ["/app/portfolio-server"]

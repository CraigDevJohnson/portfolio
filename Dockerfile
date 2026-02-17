FROM golang:1.24-alpine AS builder

WORKDIR /src

# Install templ CLI for code generation
RUN go install github.com/a-h/templ/cmd/templ@latest

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate templ components (required since *_templ.go files are gitignored)
RUN templ generate
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/portfolio-server .

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/portfolio-server /app/portfolio-server
COPY --from=builder /src/static /app/static

EXPOSE 8080

ENTRYPOINT ["/app/portfolio-server"]

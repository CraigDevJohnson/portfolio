FROM golang:1.23-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/portfolio-server .

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/portfolio-server /app/portfolio-server
COPY --from=builder /src/static /app/static

EXPOSE 8080

ENTRYPOINT ["/app/portfolio-server"]

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/core"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"portfolio/internal/app"
	"portfolio/internal/httpx"
)

const lambdaInitializationTimeout = 8 * time.Second

type proxyV2 interface {
	ProxyWithContext(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)
}

type lambdaHandlerFunc func(context.Context, *events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)

func initializeLambda(ctx context.Context) (proxyV2, error) {
	if err := resolveSSMSecrets(ctx); err != nil {
		return nil, fmt.Errorf("resolve SSM secrets: %w", err)
	}
	handler, err := app.NewLambdaHandler(ctx)
	if err != nil {
		return nil, fmt.Errorf("construct application: %w", err)
	}
	return httpadapter.NewV2(withAPIGatewayOrigin(handler)), nil
}

func newLambdaHandler(proxy proxyV2) lambdaHandlerFunc {
	return func(ctx context.Context, request *events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		if request == nil {
			return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusBadRequest, Body: "invalid request"}, nil
		}
		return proxy.ProxyWithContext(ctx, *request)
	}
}

func withAPIGatewayOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayContext, ok := core.GetAPIGatewayV2ContextFromContext(r.Context())
		domain := strings.TrimSpace(gatewayContext.DomainName)
		if !ok || domain == "" {
			http.Error(w, "gateway request context missing", http.StatusInternalServerError)
			return
		}
		r = httpx.WithTrustedOrigin(r, httpx.TrustedOrigin{Scheme: "https", Host: domain})
		next.ServeHTTP(w, r)
	})
}

func main() {
	initCtx, cancel := context.WithTimeout(context.Background(), lambdaInitializationTimeout)
	proxy, err := initializeLambda(initCtx)
	cancel()
	if err != nil {
		slog.Error("lambda initialization failed", slog.Any("error", err))
		os.Exit(1)
	}
	lambda.Start(newLambdaHandler(proxy))
}

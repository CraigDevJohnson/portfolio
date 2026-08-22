package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"portfolio/internal/app"
)

var (
	adapterMu sync.Mutex
	adapter   *httpadapter.HandlerAdapterV2
)

func lambdaHandler(ctx context.Context, req *events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	adapterMu.Lock()
	if adapter == nil {
		if err := resolveSSMSecrets(ctx); err != nil {
			adapterMu.Unlock()
			slog.Error("lambda secret resolution failed", slog.Any("error", err))
			return events.APIGatewayV2HTTPResponse{
				StatusCode: 500,
				Body:       "server initialization failed",
			}, nil
		}
		httpHandler, err := app.NewLambdaHandler()
		if err != nil {
			adapterMu.Unlock()
			slog.Error("lambda handler construction failed", slog.Any("error", err))
			return events.APIGatewayV2HTTPResponse{
				StatusCode: 500,
				Body:       "server initialization failed",
			}, nil
		}
		adapter = httpadapter.NewV2(httpHandler)
	}
	a := adapter
	adapterMu.Unlock()

	if req == nil {
		slog.Error("lambda request payload missing")
		return events.APIGatewayV2HTTPResponse{
			StatusCode: 400,
			Body:       "invalid request",
		}, nil
	}

	return a.ProxyWithContext(ctx, *req)
}

func main() {
	lambda.Start(lambdaHandler)
}

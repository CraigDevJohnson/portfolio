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
	handlerOnce sync.Once
	adapter     *httpadapter.HandlerAdapterV2
	initErr     error
)

func lambdaHandler(ctx context.Context, req *events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	handlerOnce.Do(func() {
		if err := resolveSSMSecrets(ctx); err != nil {
			initErr = err
			return
		}
		httpHandler, err := app.NewLambdaHandler()
		if err != nil {
			initErr = err
			return
		}
		adapter = httpadapter.NewV2(httpHandler)
	})

	if initErr != nil {
		slog.Error("lambda initialization failed", slog.Any("error", initErr))
		return events.APIGatewayV2HTTPResponse{
			StatusCode: 500,
			Body:       "server initialization failed",
		}, nil
	}

	if req == nil {
		slog.Error("lambda request payload missing")
		return events.APIGatewayV2HTTPResponse{
			StatusCode: 400,
			Body:       "invalid request",
		}, nil
	}

	return adapter.ProxyWithContext(ctx, *req)
}

func main() {
	lambda.Start(lambdaHandler)
}

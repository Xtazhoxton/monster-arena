// Command lambda is the AWS Lambda entry point of the catalog service.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/dynamo"
	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/handler"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	table := os.Getenv("TABLE_NAME")
	if table == "" {
		logger.Error("TABLE_NAME is not set")
		os.Exit(1)
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Error("load AWS configuration", slog.Any("error", err))
		os.Exit(1)
	}

	store := dynamo.New(dynamodb.NewFromConfig(cfg), table)

	lambda.Start(handler.New(store, logger).Handle)
}

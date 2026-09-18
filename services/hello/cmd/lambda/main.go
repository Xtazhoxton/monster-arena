// Command lambda is the AWS Lambda entry point of the hello service.
package main

import (
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/Xtazhoxton/monster-arena/services/hello/internal/handler"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	lambda.Start(handler.New(logger).Handle)
}

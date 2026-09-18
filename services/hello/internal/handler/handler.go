// Package handler contains the HTTP logic of the hello service, independent of the Lambda runtime.
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
)

type response struct {
	Message string `json:"message"`
}

// Handler serves the hello endpoint.
type Handler struct {
	logger *slog.Logger
}

// New returns a Handler that logs with logger.
func New(logger *slog.Logger) *Handler {
	return &Handler{logger: logger}
}

// Handle greets the caller named by the "name" query parameter, or "trainer" by default.
func (h *Handler) Handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	name := req.QueryStringParameters["name"]
	if name == "" {
		name = "trainer"
	}

	h.logger.InfoContext(ctx, "greeting",
		slog.String("name", name), slog.String("request_id", req.RequestContext.RequestID),
	)

	body, err := json.Marshal(response{Message: "Hello " + name + ", welcome to monster-arena"})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{}, err
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}, nil
}

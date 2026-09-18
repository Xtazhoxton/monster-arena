// Package handler contains the HTTP logic of the hello service, independent of the Lambda runtime
package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandle(t *testing.T) {
	tests := []struct {
		name        string
		query       map[string]string
		wantMessage string
	}{
		{
			name:        "default name when query is empty",
			query:       nil,
			wantMessage: "Hello trainer, welcome to monster-arena",
		},
		{
			name:        "uses the name query parameter",
			query:       map[string]string{"name": "Sacha"},
			wantMessage: "Hello Sacha, welcome to monster-arena",
		},
	}
	h := New(slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{QueryStringParameters: tt.query})
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
			}

			var got response
			if err := json.Unmarshal([]byte(resp.Body), &got); err != nil {
				t.Fatalf("invalid JSON body %q: %v", resp.Body, err)
			}
			if got.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMessage)
			}
		})
	}
}

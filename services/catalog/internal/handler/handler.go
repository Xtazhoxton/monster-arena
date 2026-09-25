// Package handler serves the catalog over HTTP, independent of the Lambda runtime.
package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/aws/aws-lambda-go/events"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// creatureStore is the slice of the repository the handlers actually need.
type creatureStore interface {
	GetCreature(ctx context.Context, id string) (catalog.Creature, bool, error)
	ListCreatures(ctx context.Context, limit int32, token string) (catalog.CreaturePage, error)
	ListCreaturesByType(ctx context.Context, typeID string, limit int32, token string) (catalog.CreaturePage, error)
	PutCreature(ctx context.Context, c catalog.Creature) error
}

// Handler serves the creature routes of the catalog.
type Handler struct {
	store  creatureStore
	logger *slog.Logger
}

// New returns a Handler reading and writing through store, logging with logger.
func New(store creatureStore, logger *slog.Logger) *Handler {
	return &Handler{store: store, logger: logger}
}

// getCreature serves GET /creatures/{id}.
func (h *Handler) getCreature(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	id := req.PathParameters["id"]
	creature, found, err := h.store.GetCreature(ctx, id)
	if err != nil {
		return h.writeStoreError(ctx, err)
	}

	if !found {
		return h.writeError(ctx, http.StatusNotFound, "creature not found")
	}

	return h.writeJSON(ctx, http.StatusOK, creature)
}

// maxLimit is the largest page this API serves. It matches the 100-item ceiling of a
// DynamoDB BatchGetItem, on which the type filter relies.
const maxLimit = 100

// parseLimit turns the "limit" query parameter into a page size. Zero means unset,
// which lets the store apply its default.
func parseLimit(raw string) (int32, error) {
	if raw == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxLimit {
		return 0, fmt.Errorf("limit must be an integer between 1 and %d", maxLimit)
	}

	return int32(n), nil
}

// listCreatures serves GET /creatures, optionally filtered by type.
func (h *Handler) listCreatures(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	limit, err := parseLimit(req.QueryStringParameters["limit"])
	if err != nil {
		return h.writeError(ctx, http.StatusBadRequest, err.Error())
	}

	cursor := req.QueryStringParameters["cursor"]

	var page catalog.CreaturePage

	if typeID := req.QueryStringParameters["type"]; typeID != "" {
		page, err = h.store.ListCreaturesByType(ctx, typeID, limit, cursor)
	} else {
		page, err = h.store.ListCreatures(ctx, limit, cursor)
	}
	if err != nil {
		return h.writeStoreError(ctx, err)
	}

	if page.Creatures == nil {
		page.Creatures = []catalog.Creature{}
	}
	return h.writeJSON(ctx, http.StatusOK, page)
}

// requestBody returns the body of req, decoded when API Gateway sent it base64-encoded.
func requestBody(req events.APIGatewayV2HTTPRequest) ([]byte, error) {
	if !req.IsBase64Encoded {
		return []byte(req.Body), nil
	}
	return base64.StdEncoding.DecodeString(req.Body)
}

// putCreature serves PUT /creatures/{id}. The URL names the resource being written;
// a body that disagrees with it is rejected, never silently overwritten.
func (h *Handler) putCreature(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	body, err := requestBody(req)
	if err != nil {
		return h.writeError(ctx, http.StatusBadRequest, "body is not valid base64")
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	var creature catalog.Creature
	if err := dec.Decode(&creature); err != nil {
		return h.writeError(ctx, http.StatusBadRequest, err.Error())
	}
	if dec.More() {
		return h.writeError(ctx, http.StatusBadRequest, "body must contain a single JSON object")
	}

	id := req.PathParameters["id"]
	if creature.ID != "" && creature.ID != id {
		return h.writeError(ctx, http.StatusBadRequest, "id in body does not match the id in the URL")
	}
	creature.ID = id
	if len(creature.Moves) > 0 {
		return h.writeError(ctx, http.StatusBadRequest, "movepool is written through PUT /creatures/{id}/moves")
	}
	if err := h.store.PutCreature(ctx, creature); err != nil {
		return h.writeStoreError(ctx, err)
	}
	return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusNoContent}
}

// Handle dispatches one request to the method serving its route.
// Every case mirrors a route declared in Terraform.
func (h *Handler) Handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	switch req.RouteKey {

	case "GET /creatures/{id}":
		return h.getCreature(ctx, req), nil

	case "GET /creatures":
		return h.listCreatures(ctx, req), nil

	case "PUT /creatures/{id}":
		return h.putCreature(ctx, req), nil

	default:
		h.logger.ErrorContext(ctx, "unrouted request", slog.String("route_key", req.RouteKey))
		return h.writeError(ctx, http.StatusNotFound, "unknown route"), nil
	}
}

// jsonHeaders returns the headers common to every response: this API speaks JSON only.
func jsonHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json"}
}

// errorBody is the single error shape of this API.
type errorBody struct {
	Error string `json:"error"`
}

// writeJSON renders status and payload as the response API Gateway expects.
func (h *Handler) writeJSON(ctx context.Context, status int, payload any) events.APIGatewayV2HTTPResponse {
	body, err := json.Marshal(payload)
	if err != nil {
		h.logger.ErrorContext(ctx, "marshal response", slog.Any("error", err))
		return events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusInternalServerError,
			Headers:    jsonHeaders(),
			Body:       `{"error":"internal error"}`,
		}
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    jsonHeaders(),
		Body:       string(body),
	}
}

// writeError answers with status and a message meant for the client.
func (h *Handler) writeError(ctx context.Context, status int, message string) events.APIGatewayV2HTTPResponse {
	return h.writeJSON(ctx, status, errorBody{Error: message})
}

// writeStoreError maps an error coming from the store to a response: invalid input is
// the caller's fault and says why, anything else is ours and says nothing.
func (h *Handler) writeStoreError(ctx context.Context, err error) events.APIGatewayV2HTTPResponse {
	if errors.Is(err, catalog.ErrInvalid) {
		h.logger.InfoContext(ctx, "rejected request", slog.Any("error", err))
		return h.writeError(ctx, http.StatusBadRequest, err.Error())
	}

	h.logger.ErrorContext(ctx, "store failure", slog.Any("error", err))
	return h.writeError(ctx, http.StatusInternalServerError, "internal error")
}

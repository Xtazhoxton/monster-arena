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

// creatureStore is the slice of the repository the creature routes need.
type creatureStore interface {
	GetCreature(ctx context.Context, id string) (catalog.Creature, bool, error)
	ListCreatures(ctx context.Context, limit int32, token string) (catalog.CreaturePage, error)
	ListCreaturesByType(ctx context.Context, typeID string, limit int32, token string) (catalog.CreaturePage, error)
	PutCreature(ctx context.Context, c catalog.Creature) error
	PutMovepool(ctx context.Context, id string, p catalog.Movepool) (bool, error)
}

// moveStore is the slice of the repository the move routes need.
type moveStore interface {
	GetMove(ctx context.Context, id string) (catalog.Move, bool, error)
	ListMoves(ctx context.Context, limit int32, token string) (catalog.MovePage, error)
	PutMove(ctx context.Context, m catalog.Move) error
}

// typeStore is the slice of the repository the type routes need.
type typeStore interface {
	GetType(ctx context.Context, id string) (catalog.Type, bool, error)
	ListTypes(ctx context.Context, limit int32, token string) (catalog.TypePage, error)
	PutType(ctx context.Context, t catalog.Type) error
}

// catalogStore is everything the handler needs, whoever provides it.
type catalogStore interface {
	creatureStore
	moveStore
	typeStore
}

// Handler serves the catalog routes.
type Handler struct {
	store  catalogStore
	logger *slog.Logger
}

// New returns a Handler reading and writing through store, logging with logger.
func New(store catalogStore, logger *slog.Logger) *Handler {
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

// getMove serves GET /moves/{id}.
func (h *Handler) getMove(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	id := req.PathParameters["id"]

	move, found, err := h.store.GetMove(ctx, id)
	if err != nil {
		return h.writeStoreError(ctx, err)
	}

	if !found {
		return h.writeError(ctx, http.StatusNotFound, "move not found")
	}

	return h.writeJSON(ctx, http.StatusOK, move)
}

// getType serves GET /types/{id}.
func (h *Handler) getType(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	id := req.PathParameters["id"]

	elementalType, found, err := h.store.GetType(ctx, id)
	if err != nil {
		return h.writeStoreError(ctx, err)
	}

	if !found {
		return h.writeError(ctx, http.StatusNotFound, "type not found")
	}

	return h.writeJSON(ctx, http.StatusOK, elementalType)
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

// listMoves serves GET /moves.
func (h *Handler) listMoves(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	limit, err := parseLimit(req.QueryStringParameters["limit"])
	if err != nil {
		return h.writeError(ctx, http.StatusBadRequest, err.Error())
	}

	page, err := h.store.ListMoves(ctx, limit, req.QueryStringParameters["cursor"])
	if err != nil {
		return h.writeStoreError(ctx, err)
	}

	if page.Moves == nil {
		page.Moves = []catalog.Move{}
	}

	return h.writeJSON(ctx, http.StatusOK, page)
}

// listTypes serves GET /types.
func (h *Handler) listTypes(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	limit, err := parseLimit(req.QueryStringParameters["limit"])
	if err != nil {
		return h.writeError(ctx, http.StatusBadRequest, err.Error())
	}

	page, err := h.store.ListTypes(ctx, limit, req.QueryStringParameters["cursor"])
	if err != nil {
		return h.writeStoreError(ctx, err)
	}

	if page.Types == nil {
		page.Types = []catalog.Type{}
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

// putMovepool serves PUT /creatures/{id}/moves. The body replaces the whole movepool,
// so the stored representation is known and returned.
func (h *Handler) putMovepool(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	body, err := requestBody(req)
	if err != nil {
		return h.writeError(ctx, http.StatusBadRequest, "body is not valid base64")
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	var movepool catalog.Movepool
	if err := dec.Decode(&movepool); err != nil {
		return h.writeError(ctx, http.StatusBadRequest, err.Error())
	}
	if dec.More() {
		return h.writeError(ctx, http.StatusBadRequest, "body must contain a single JSON object")
	}

	if movepool.Moves == nil {
		return h.writeError(ctx, http.StatusBadRequest, "moves is required; send [] to empty the movepool")
	}

	found, err := h.store.PutMovepool(ctx, req.PathParameters["id"], movepool)
	if err != nil {
		return h.writeStoreError(ctx, err)
	}
	if !found {
		return h.writeError(ctx, http.StatusNotFound, "creature not found")
	}
	return h.writeJSON(ctx, http.StatusOK, movepool)
}

// putMove serves PUT /moves/{id}. A move has no sub-resource, so the body is the
// whole entity: the stored representatio is known and return.
func (h *Handler) putMove(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	body, err := requestBody(req)
	if err != nil {
		return h.writeError(ctx, http.StatusBadRequest, "body is not valid base64")
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	var move catalog.Move
	if err := dec.Decode(&move); err != nil {
		return h.writeError(ctx, http.StatusBadRequest, err.Error())
	}
	if dec.More() {
		return h.writeError(ctx, http.StatusBadRequest, "body must contain a single JSON object")
	}

	id := req.PathParameters["id"]
	if move.ID != "" && move.ID != id {
		return h.writeError(ctx, http.StatusBadRequest, "id in body does not match the id in the URL")
	}
	move.ID = id
	if err := h.store.PutMove(ctx, move); err != nil {
		return h.writeStoreError(ctx, err)
	}

	return h.writeJSON(ctx, http.StatusOK, move)
}

// putType serves PUT /types/{id}. Like a move, a type is written whole — but the stored
// effectiveness map is the normalised one, so the response echoes the store's view.
func (h *Handler) putType(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	body, err := requestBody(req)
	if err != nil {
		return h.writeError(ctx, http.StatusBadRequest, "body is not valid base64")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	var elementalType catalog.Type
	if err := dec.Decode(&elementalType); err != nil {
		return h.writeError(ctx, http.StatusBadRequest, err.Error())
	}
	if dec.More() {
		return h.writeError(ctx, http.StatusBadRequest, "body must contain a single JSON object")
	}

	id := req.PathParameters["id"]
	if elementalType.ID != "" && elementalType.ID != id {
		return h.writeError(ctx, http.StatusBadRequest, "id in body does not match the id in the URL")
	}

	elementalType.ID = id
	if err := h.store.PutType(ctx, elementalType); err != nil {
		return h.writeStoreError(ctx, err)
	}

	// Presentation only, after the write: the response always carries an object, never
	// null, so a client can iterate the matrix without testing it first.
	if elementalType.Effectiveness == nil {
		elementalType.Effectiveness = map[string]float64{}
	}

	return h.writeJSON(ctx, http.StatusOK, elementalType)
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

	case "PUT /creatures/{id}/moves":
		return h.putMovepool(ctx, req), nil

	case "GET /moves/{id}":
		return h.getMove(ctx, req), nil

	case "GET /moves":
		return h.listMoves(ctx, req), nil

	case "PUT /moves/{id}":
		return h.putMove(ctx, req), nil

	case "GET /types/{id}":
		return h.getType(ctx, req), nil

	case "GET /types":
		return h.listTypes(ctx, req), nil

	case "PUT /types/{id}":
		return h.putType(ctx, req), nil

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

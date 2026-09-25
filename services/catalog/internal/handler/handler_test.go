package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// fakeStore satisfies creatureStore with behaviour injected per test. Each field is the
// function its method delegates to; a test only sets the one it expects to be called, so
// an unexpected call panics on a nil field and names the method that was not wanted.
type fakeStore struct {
	getCreature         func(ctx context.Context, id string) (catalog.Creature, bool, error)
	listCreatures       func(ctx context.Context, limit int32, token string) (catalog.CreaturePage, error)
	listCreaturesByType func(ctx context.Context, typeID string, limit int32, token string) (catalog.CreaturePage, error)
	putCreature         func(ctx context.Context, c catalog.Creature) error
}

func (f fakeStore) GetCreature(ctx context.Context, id string) (catalog.Creature, bool, error) {
	return f.getCreature(ctx, id)
}

func (f fakeStore) ListCreatures(ctx context.Context, limit int32, token string) (catalog.CreaturePage, error) {
	return f.listCreatures(ctx, limit, token)
}

func (f fakeStore) ListCreaturesByType(ctx context.Context, typeID string, limit int32, token string) (catalog.CreaturePage, error) {
	return f.listCreaturesByType(ctx, typeID, limit, token)
}

func (f fakeStore) PutCreature(ctx context.Context, c catalog.Creature) error {
	return f.putCreature(ctx, c)
}

// newTestHandler returns a Handler over store, logging nowhere.
func newTestHandler(store creatureStore) *Handler {
	return New(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// pikachu is the creature the tests read and write.
func pikachu() catalog.Creature {
	return catalog.Creature{
		ID:         "pikachu",
		Name:       "Pikachu",
		Generation: 1,
		Types:      []string{"electric"},
		BaseStats: catalog.Stats{
			HP: 35, Attack: 55, Defense: 40,
			SpecialAttack: 50, SpecialDefense: 50, Speed: 90,
		},
	}
}

// The wire format is frozen literally: these strings are the API contract, and a change
// to a JSON tag or to the field order must show up here as a failing test.
const (
	pikachuJSON = `{"id":"pikachu","name":"Pikachu","generation":1,"types":["electric"],` +
		`"baseStats":{"hp":35,"attack":55,"defense":40,"specialAttack":50,"specialDefense":50,"speed":90}}`

	pikachuWithMovesJSON = `{"id":"pikachu","name":"Pikachu","generation":1,"types":["electric"],` +
		`"baseStats":{"hp":35,"attack":55,"defense":40,"specialAttack":50,"specialDefense":50,"speed":90},` +
		`"moves":[{"id":"thunderbolt","learnMethod":"level-up","level":26}]}`

	// pikachuBody is a valid PUT body: no "id", since the URL carries it.
	pikachuBody = `{"name":"Pikachu","generation":1,"types":["electric"],` +
		`"baseStats":{"hp":35,"attack":55,"defense":40,"specialAttack":50,"specialDefense":50,"speed":90}}`
)

func TestParseLimit(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int32
		wantErr bool
	}{
		{name: "unset means zero", raw: "", want: 0},
		{name: "in range", raw: "20", want: 20},
		{name: "lower bound", raw: "1", want: 1},
		{name: "upper bound", raw: "100", want: 100},
		{name: "zero is refused", raw: "0", wantErr: true},
		{name: "negative is refused", raw: "-5", wantErr: true},
		{name: "above the maximum is refused", raw: "101", wantErr: true},
		{name: "not a number", raw: "abc", wantErr: true},
		{name: "not an integer", raw: "20.5", wantErr: true},
		{name: "blank is not empty", raw: " ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLimit(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseLimit(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseLimit(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestHandleUnknownRoute(t *testing.T) {
	h := newTestHandler(fakeStore{})

	resp, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{
		RouteKey: "DELETE /creatures/{id}",
	})
	if err != nil {
		t.Fatalf("Handle returned error %v, want nil", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if want := `{"error":"unknown route"}`; resp.Body != want {
		t.Errorf("body = %s, want %s", resp.Body, want)
	}
}

func TestHandleGetCreature(t *testing.T) {
	tests := []struct {
		name       string
		get        func(ctx context.Context, id string) (catalog.Creature, bool, error)
		wantStatus int
		wantBody   string
	}{
		{
			name: "found, movepool included",
			get: func(context.Context, string) (catalog.Creature, bool, error) {
				c := pikachu()
				c.Moves = []catalog.LearnedMove{
					{MoveID: "thunderbolt", LearnMethod: "level-up", Level: 26},
				}
				return c, true, nil
			},
			wantStatus: http.StatusOK,
			wantBody:   pikachuWithMovesJSON,
		},
		{
			name: "absent is not a failure",
			get: func(context.Context, string) (catalog.Creature, bool, error) {
				return catalog.Creature{}, false, nil
			},
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"creature not found"}`,
		},
		{
			// A failure of ours must never reach the client: the table name, the region and
			// the account id in that message are reconnaissance offered to an attacker.
			name: "store failure stays internal",
			get: func(context.Context, string) (catalog.Creature, bool, error) {
				return catalog.Creature{}, false, errors.New("ResourceNotFoundException: table monster-arena-catalog-dev, account 533449297933")
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"internal error"}`,
		},
		{
			name: "invalid input is explained",
			get: func(context.Context, string) (catalog.Creature, bool, error) {
				return catalog.Creature{}, false, fmt.Errorf("%w: id %q must be a slug", catalog.ErrInvalid, "NOPE")
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid: id \"NOPE\" must be a slug"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(fakeStore{getCreature: tt.get})

			resp, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{
				RouteKey:       "GET /creatures/{id}",
				PathParameters: map[string]string{"id": "pikachu"},
			})
			if err != nil {
				t.Fatalf("Handle returned error %v, want nil", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if resp.Body != tt.wantBody {
				t.Errorf("body  = %s\nwant  = %s", resp.Body, tt.wantBody)
			}
			if got := resp.Headers["Content-Type"]; got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
		})
	}
}

func TestHandleListCreatures(t *testing.T) {
	tests := []struct {
		name       string
		query      map[string]string
		page       catalog.CreaturePage
		pageErr    error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "one page, more to come",
			page:       catalog.CreaturePage{Creatures: []catalog.Creature{pikachu()}, NextCursor: "eyJwayI6ICJ4In0"},
			wantStatus: http.StatusOK,
			wantBody:   `{"creatures":[` + pikachuJSON + `],"nextCursor":"eyJwayI6ICJ4In0"}`,
		},
		{
			// Last page: nextCursor must disappear, not come back empty. Its presence is
			// how a client knows whether to ask for more.
			name:       "last page omits the cursor",
			page:       catalog.CreaturePage{Creatures: []catalog.Creature{pikachu()}},
			wantStatus: http.StatusOK,
			wantBody:   `{"creatures":[` + pikachuJSON + `]}`,
		},
		{
			// A nil slice marshals to null, which breaks any client looping over the result.
			// The store returns nil when a filter matches nothing, so this is a real case.
			name:       "empty page is an array, never null",
			page:       catalog.CreaturePage{},
			wantStatus: http.StatusOK,
			wantBody:   `{"creatures":[]}`,
		},
		{
			name:       "store failure stays internal",
			pageErr:    errors.New("ProvisionedThroughputExceededException"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"internal error"}`,
		},
		{
			name:       "invalid cursor is the caller's fault",
			pageErr:    fmt.Errorf("%w: cursor is not valid base64", catalog.ErrInvalid),
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid: cursor is not valid base64"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(fakeStore{
				listCreatures: func(context.Context, int32, string) (catalog.CreaturePage, error) {
					return tt.page, tt.pageErr
				},
			})

			resp, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{
				RouteKey:              "GET /creatures",
				QueryStringParameters: tt.query,
			})
			if err != nil {
				t.Fatalf("Handle returned error %v, want nil", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if resp.Body != tt.wantBody {
				t.Errorf("body  = %s\nwant  = %s", resp.Body, tt.wantBody)
			}
		})
	}
}

// TestHandleListCreaturesForwardsParameters checks what the handler passes down: an unset
// limit stays 0 so the store applies its own default, and the cursor travels untouched.
func TestHandleListCreaturesForwardsParameters(t *testing.T) {
	tests := []struct {
		name       string
		query      map[string]string
		wantLimit  int32
		wantCursor string
	}{
		{name: "no parameter", wantLimit: 0, wantCursor: ""},
		{name: "limit only", query: map[string]string{"limit": "50"}, wantLimit: 50},
		{
			name:       "limit and cursor",
			query:      map[string]string{"limit": "1", "cursor": "eyJwayI6ICJ4In0"},
			wantLimit:  1,
			wantCursor: "eyJwayI6ICJ4In0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotLimit int32
			var gotCursor string

			h := newTestHandler(fakeStore{
				listCreatures: func(_ context.Context, limit int32, token string) (catalog.CreaturePage, error) {
					gotLimit, gotCursor = limit, token
					return catalog.CreaturePage{}, nil
				},
			})

			if _, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{
				RouteKey:              "GET /creatures",
				QueryStringParameters: tt.query,
			}); err != nil {
				t.Fatalf("Handle returned error %v, want nil", err)
			}
			if gotLimit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", gotLimit, tt.wantLimit)
			}
			if gotCursor != tt.wantCursor {
				t.Errorf("cursor = %q, want %q", gotCursor, tt.wantCursor)
			}
		})
	}
}

// TestHandleListCreaturesByType checks the branch: a "type" parameter must reach
// ListCreaturesByType, and ListCreatures must not be called at all.
func TestHandleListCreaturesByType(t *testing.T) {
	var gotType string

	h := newTestHandler(fakeStore{
		listCreaturesByType: func(_ context.Context, typeID string, _ int32, _ string) (catalog.CreaturePage, error) {
			gotType = typeID
			return catalog.CreaturePage{Creatures: []catalog.Creature{pikachu()}}, nil
		},
		// listCreatures stays nil: calling it would panic and fail this test.
	})

	resp, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{
		RouteKey:              "GET /creatures",
		QueryStringParameters: map[string]string{"type": "electric"},
	})
	if err != nil {
		t.Fatalf("Handle returned error %v, want nil", err)
	}
	if gotType != "electric" {
		t.Errorf("typeID = %q, want electric", gotType)
	}
	if want := `{"creatures":[` + pikachuJSON + `]}`; resp.Body != want {
		t.Errorf("body  = %s\nwant  = %s", resp.Body, want)
	}
}

// TestHandleListCreaturesRejectsBadLimit also checks that nothing reaches the store: the
// nil listCreatures field would panic if the handler queried it anyway.
func TestHandleListCreaturesRejectsBadLimit(t *testing.T) {
	h := newTestHandler(fakeStore{})

	resp, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{
		RouteKey:              "GET /creatures",
		QueryStringParameters: map[string]string{"limit": "1000"},
	})
	if err != nil {
		t.Fatalf("Handle returned error %v, want nil", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if want := `{"error":"limit must be an integer between 1 and 100"}`; resp.Body != want {
		t.Errorf("body = %s, want %s", resp.Body, want)
	}
}

func TestHandlePutCreature(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		base64Body bool
		putErr     error
		wantStatus int
		wantBody   string
		wantStored bool
	}{
		{
			name:       "valid body, id taken from the URL",
			body:       pikachuBody,
			wantStatus: http.StatusNoContent,
			wantStored: true,
		},
		{
			name:       "id in body agreeing with the URL",
			body:       `{"id":"pikachu",` + pikachuBody[1:],
			wantStatus: http.StatusNoContent,
			wantStored: true,
		},
		{
			// Silently overwriting would let a buggy import script crush one creature
			// while believing it wrote a thousand.
			name:       "id in body disagreeing with the URL",
			body:       `{"id":"raichu",` + pikachuBody[1:],
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"id in body does not match the id in the URL"}`,
		},
		{
			// The corruption this guards against: encoding/json ignores unknown fields by
			// default, so this typo would store a creature whose every stat is zero.
			name:       "misspelled field is refused",
			body:       `{"name":"Pikachu","types":["electric"],"baseStat":{"hp":35}}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"json: unknown field \"baseStat\""}`,
		},
		{
			name:       "movepool belongs to its own route",
			body:       `{"name":"Pikachu","types":["electric"],"baseStats":{"hp":35},"moves":[{"id":"thunderbolt","learnMethod":"level-up"}]}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"movepool is written through PUT /creatures/{id}/moves"}`,
		},
		{
			name:       "two objects in one body",
			body:       pikachuBody + `{"name":"Raichu"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"body must contain a single JSON object"}`,
		},
		{
			name:       "base64 body is decoded",
			body:       base64.StdEncoding.EncodeToString([]byte(pikachuBody)),
			base64Body: true,
			wantStatus: http.StatusNoContent,
			wantStored: true,
		},
		{
			name:       "body announced as base64 but is not",
			body:       "!!!not base64!!!",
			base64Body: true,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"body is not valid base64"}`,
		},
		{
			name:       "domain rules are the store's to enforce",
			body:       pikachuBody,
			putErr:     fmt.Errorf("%w: want 1 or 2 types, got 0", catalog.ErrInvalid),
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid: want 1 or 2 types, got 0"}`,
			wantStored: true,
		},
		{
			name:       "store failure stays internal",
			body:       pikachuBody,
			putErr:     errors.New("TransactionCanceledException: ConditionalCheckFailed"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"internal error"}`,
			wantStored: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stored *catalog.Creature

			h := newTestHandler(fakeStore{
				putCreature: func(_ context.Context, c catalog.Creature) error {
					stored = &c
					return tt.putErr
				},
			})

			resp, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{
				RouteKey:        "PUT /creatures/{id}",
				PathParameters:  map[string]string{"id": "pikachu"},
				Body:            tt.body,
				IsBase64Encoded: tt.base64Body,
			})
			if err != nil {
				t.Fatalf("Handle returned error %v, want nil", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if resp.Body != tt.wantBody {
				t.Errorf("body  = %s\nwant  = %s", resp.Body, tt.wantBody)
			}
			switch {
			case tt.wantStored && stored == nil:
				t.Error("store was not called, want a write")
			case !tt.wantStored && stored != nil:
				t.Errorf("store was called with %+v, want no write", *stored)
			case tt.wantStored && stored.ID != "pikachu":
				t.Errorf("stored id = %q, want pikachu", stored.ID)
			}
		})
	}
}

// TestHandlePutCreatureRejectsMalformedJSON keeps the wording of encoding/json out of the
// assertions: only the status and the absence of any write are our contract.
func TestHandlePutCreatureRejectsMalformedJSON(t *testing.T) {
	for _, body := range []string{"", "{", "not json at all", `{"name":}`, `[]`} {
		t.Run(fmt.Sprintf("body %q", body), func(t *testing.T) {
			h := newTestHandler(fakeStore{
				putCreature: func(_ context.Context, c catalog.Creature) error {
					t.Errorf("store was called with %+v, want no write", c)
					return nil
				},
			})

			resp, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{
				RouteKey:       "PUT /creatures/{id}",
				PathParameters: map[string]string{"id": "pikachu"},
				Body:           body,
			})
			if err != nil {
				t.Fatalf("Handle returned error %v, want nil", err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// testAPI returns an api pointed at a server answering the given statuses in turn,
// and the number of requests the server received.
func testAPI(t *testing.T, statuses ...int) (*api, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1))

		if r.Method != http.MethodPut || r.URL.Path != "/creatures/flamkin/moves" {
			t.Errorf("request = %s %s, want PUT /creatures/flamkin/moves", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want the bearer token", got)
		}
		// Every attempt must carry the whole body, not a drained reader.
		var pool catalog.Movepool
		if err := json.NewDecoder(r.Body).Decode(&pool); err != nil || len(pool.Moves) != 1 {
			t.Errorf("attempt %d: body = %+v, %v; want the movepool", n, pool, err)
		}

		w.WriteHeader(statuses[min(n, len(statuses))-1])
		_, _ = io.WriteString(w, `{"error":"nope"}`)
	}))
	t.Cleanup(srv.Close)

	// The trailing slash mirrors the stage invoke URL.
	return &api{http: srv.Client(), base: srv.URL + "/", token: "secret"}, &calls
}

func TestPut(t *testing.T) {
	tests := []struct {
		name      string
		statuses  []int
		wantCalls int32
		wantErr   bool
		wantAuth  bool
	}{
		{name: "success", statuses: []int{http.StatusOK}, wantCalls: 1},
		{name: "throttled then accepted", statuses: []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusNoContent}, wantCalls: 3},
		{name: "server error on every attempt", statuses: []int{http.StatusInternalServerError}, wantCalls: maxAttempts, wantErr: true},
		{name: "validation refusal is not retried", statuses: []int{http.StatusBadRequest}, wantCalls: 1, wantErr: true},
		{name: "expired token stops the import", statuses: []int{http.StatusUnauthorized}, wantCalls: 1, wantErr: true, wantAuth: true},
		{name: "missing scope stops the import", statuses: []int{http.StatusForbidden}, wantCalls: 1, wantErr: true, wantAuth: true},
	}

	pool := catalog.Movepool{Moves: []catalog.LearnedMove{{MoveID: "ember", LearnMethod: catalog.LevelUp, Level: 7}}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, calls := testAPI(t, tt.statuses...)

			err := a.put(context.Background(), pool, "creatures", "flamkin", "moves")

			if (err != nil) != tt.wantErr {
				t.Errorf("put() error = %v, want error: %v", err, tt.wantErr)
			}
			if got := errors.Is(err, errUnauthorized); got != tt.wantAuth {
				t.Errorf("put() error = %v, wrapping errUnauthorized: %v, want %v", err, got, tt.wantAuth)
			}
			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("server received %d requests, want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestKeepImported(t *testing.T) {
	imported := map[string]bool{"ember": true}
	isImported := func(id string) bool { return imported[id] }

	tests := []struct {
		name        string
		moves       []catalog.LearnedMove
		want        []catalog.LearnedMove
		wantDropped int
	}{
		{
			name:        "missing moves are dropped",
			moves:       []catalog.LearnedMove{{MoveID: "ember", LearnMethod: "machine"}, {MoveID: "z-move", LearnMethod: "machine"}},
			want:        []catalog.LearnedMove{{MoveID: "ember", LearnMethod: "machine"}},
			wantDropped: 1,
		},
		{
			name:        "dropping everything leaves an empty movepool, not a null one",
			moves:       []catalog.LearnedMove{{MoveID: "z-move", LearnMethod: "machine"}},
			want:        []catalog.LearnedMove{},
			wantDropped: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, dropped := keepImported(catalog.Movepool{Moves: tt.moves}, isImported)

			if !reflect.DeepEqual(got.Moves, tt.want) || dropped != tt.wantDropped {
				t.Errorf("keepImported() = %#v, %d; want %#v, %d", got.Moves, dropped, tt.want, tt.wantDropped)
			}
		})
	}
}

func TestRecord(t *testing.T) {
	r := newReport()
	ctx := context.Background()

	if err := r.record(ctx, kindMoves, "ember", nil); err != nil {
		t.Errorf("record(success) = %v, want nil", err)
	}
	if err := r.record(ctx, kindMoves, "z-move", errors.New("400")); err != nil {
		t.Errorf("record(refusal) = %v, want nil: a refusal must not stop the import", err)
	}
	if err := r.record(ctx, kindMoves, "surf", errUnauthorized); !errors.Is(err, errUnauthorized) {
		t.Errorf("record(unauthorized) = %v, want errUnauthorized: it must stop the import", err)
	}

	if !r.imported(kindMoves, "ember") || r.imported(kindMoves, "z-move") || r.imported(kindMoves, "surf") {
		t.Errorf("imported moves = %v, want only ember", r.written[kindMoves])
	}
	if !reflect.DeepEqual(r.refused, []string{"moves/z-move"}) {
		t.Errorf("refused = %v, want [moves/z-move]", r.refused)
	}
}

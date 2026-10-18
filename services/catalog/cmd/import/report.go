package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
)

// report collects the outcome of every write. Workers run concurrently, hence the mutex.
type report struct {
	mu sync.Mutex
	// written maps a kind of entity ("types", "moves"…) to the ids the API accepted.
	written map[string]map[string]bool
	refused []string
	// skipped counts the moves left out on purpose: Z-moves and shadow moves (ADR-0012).
	skipped int
	// dropped counts movepool entries left out because their move was not imported.
	dropped int
}

func newReport() *report {
	return &report{written: make(map[string]map[string]bool)}
}

// record notes the outcome of writing one entity. It returns an error only when the
// whole import must stop: a refused token, or an interruption (Ctrl-C).
func (r *report) record(ctx context.Context, kind, id string, err error) error {
	if err != nil && (errors.Is(err, errUnauthorized) || ctx.Err() != nil) {
		return err
	}

	// syntaxe : Lock / defer Unlock : un seul worker à la fois modifie les maps et la
	// syntaxe : slice. Le defer garantit le Unlock quel que soit le chemin de sortie.
	r.mu.Lock()
	defer r.mu.Unlock()

	if err != nil {
		log.Printf("refused %s %s: %v", kind, id, err)
		r.refused = append(r.refused, kind+"/"+id)
		return nil
	}
	if r.written[kind] == nil {
		r.written[kind] = make(map[string]bool)
	}
	r.written[kind][id] = true

	return nil
}

// imported reports whether the API accepted the entity kind/id.
func (r *report) imported(kind, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.written[kind][id]
}

func (r *report) skip(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.skipped += n
}

func (r *report) drop(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.dropped += n
}

// print writes the summary; it runs after every worker is done, so it takes no lock.
func (r *report) print(w io.Writer) {
	for _, kind := range []string{kindTypes, kindMoves, kindCreatures, kindMovepools} {
		_, _ = fmt.Fprintf(w, "%-10s %4d written\n", kind, len(r.written[kind]))
	}
	_, _ = fmt.Fprintf(w, "moves skipped (Z-moves, shadow moves): %d\n", r.skipped)
	_, _ = fmt.Fprintf(w, "movepool entries dropped (move not imported): %d\n", r.dropped)
	_, _ = fmt.Fprintf(w, "refused: %d\n", len(r.refused))
	for _, entity := range r.refused {
		_, _ = fmt.Fprintf(w, "  %s\n", entity)
	}
}

// Command import fills the catalog from PokéAPI, writing through the catalog API like
// any other client (ADR-0012). From the repository root:
//
//	url=$(terraform -chdir=infra/envs/dev output -raw creatures_url)
//	API_URL=${url%creatures} TOKEN=$(make -s token) go run ./services/catalog/cmd/import
//
// PokéAPI responses are cached in the user cache directory: a rerun costs PokéAPI
// nothing, and every write is an idempotent PUT, so a failed import is simply rerun.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/pokeapi"
)

// concurrency bounds the writes in flight: under the 20 req/s of the PUT routes and far
// from the 10 concurrent Lambda executions of the account (ADR-0012).
const concurrency = 3

const (
	kindTypes     = "types"
	kindMoves     = "moves"
	kindCreatures = "creatures"
	kindMovepools = "movepools"
)

func main() {
	// pourquoi : Ctrl-C annule ctx au lieu de tuer le programme net : les requêtes en
	// pourquoi : vol s'arrêtent proprement et le bilan de ce qui a été écrit s'affiche.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx); err != nil {
		log.Print(err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	token, base := os.Getenv("TOKEN"), os.Getenv("API_URL")
	if token == "" || base == "" {
		return errors.New("TOKEN and API_URL must be set: see go doc ./services/catalog/cmd/import")
	}

	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	poke, err := pokeapi.New(filepath.Join(cacheRoot, "monster-arena", "pokeapi"))
	if err != nil {
		return err
	}

	imp := &importer{
		poke:   poke,
		api:    &api{http: &http.Client{Timeout: 30 * time.Second}, base: base, token: token, pause: time.Second},
		report: newReport(),
	}

	// pourquoi : l'ordre compte : chaque phase ne référence que ce que les précédentes
	// pourquoi : ont écrit. On n'enchaîne donc qu'une fois la phase entière terminée.
	phases := []func(context.Context) error{imp.importTypes, imp.importMoves, imp.importCreatures}
	for _, phase := range phases {
		if err := phase(ctx); err != nil {
			// pourquoi : même interrompu, on montre ce qui a été écrit avant l'arrêt.
			imp.report.print(os.Stdout)
			return err
		}
	}

	imp.report.print(os.Stdout)
	if n := len(imp.report.refused); n > 0 {
		return fmt.Errorf("%d entities refused, see the log above", n)
	}

	return nil
}

type importer struct {
	poke   *pokeapi.Client
	api    *api
	report *report
}

// forEach runs fn on every id, at most concurrency at a time, and waits for them all.
// The first error fn returns cancels the others: fn returns one only to stop the import.
func forEach(ctx context.Context, ids []string, fn func(ctx context.Context, id string) error) error {
	// pourquoi : WithContext donne un ctx annulé dès qu'un worker renvoie une erreur :
	// pourquoi : un token refusé arrête tout le monde, pas seulement ce worker.
	g, ctx := errgroup.WithContext(ctx)
	// syntaxe : SetLimit fait attendre g.Go tant que concurrency workers tournent déjà.
	g.SetLimit(concurrency)

	for _, id := range ids {
		g.Go(func() error {
			// pourquoi : une fois annulé, inutile de lancer les ids restants.
			if err := ctx.Err(); err != nil {
				return err
			}
			return fn(ctx, id)
		})
	}

	return g.Wait()
}

func (imp *importer) importTypes(ctx context.Context) error {
	names, err := imp.poke.BattleTypeNames(ctx)
	if err != nil {
		return err
	}
	log.Printf("importing %d types", len(names))

	return forEach(ctx, names, func(ctx context.Context, name string) error {
		t, err := imp.poke.Type(ctx, name)
		if err == nil {
			err = imp.api.put(ctx, t, kindTypes, t.ID)
		}
		return imp.report.record(ctx, kindTypes, name, err)
	})
}

func (imp *importer) importMoves(ctx context.Context) error {
	names, err := imp.poke.List(ctx, "move")
	if err != nil {
		return err
	}

	// pourquoi : filtrer à la source ce que notre API refuserait à coup sûr et qu'aucun
	// pourquoi : movepool n'utilise (les capacités Z, en « -- ») : un refus reste un signal.
	all := len(names)
	names = slices.DeleteFunc(names, func(name string) bool { return !catalog.IsSlug(name) })
	imp.report.skip(all - len(names))
	log.Printf("importing %d moves", len(names))

	return forEach(ctx, names, func(ctx context.Context, name string) error {
		m, err := imp.poke.Move(ctx, name)
		if errors.Is(err, pokeapi.ErrNotForBattle) {
			imp.report.skip(1)
			return nil
		}
		if err == nil {
			err = imp.api.put(ctx, m, kindMoves, m.ID)
		}
		return imp.report.record(ctx, kindMoves, name, err)
	})
}

// importCreatures writes each creature, then its movepool. A movepool only references
// its own creature and moves, all written by then, so both go in the same worker.
func (imp *importer) importCreatures(ctx context.Context) error {
	orders, err := imp.poke.VersionGroupOrders(ctx)
	if err != nil {
		return err
	}
	names, err := imp.poke.List(ctx, "pokemon-species")
	if err != nil {
		return err
	}
	log.Printf("importing %d creatures with their movepools", len(names))

	return forEach(ctx, names, func(ctx context.Context, name string) error {
		creature, pool, err := imp.poke.Creature(ctx, name, orders)
		if err == nil {
			err = imp.api.put(ctx, creature, kindCreatures, creature.ID)
		}
		if err := imp.report.record(ctx, kindCreatures, name, err); err != nil {
			return err
		}
		// pourquoi : sans sa créature, le PUT du movepool rendrait 404 : déjà compté.
		if !imp.report.imported(kindCreatures, name) {
			return nil
		}

		pool, dropped := keepImported(pool, func(id string) bool { return imp.report.imported(kindMoves, id) })
		if dropped > 0 {
			log.Printf("movepool %s: %d entries dropped, their move was not imported", name, dropped)
			imp.report.drop(dropped)
		}

		err = imp.api.put(ctx, pool, kindCreatures, creature.ID, "moves")
		return imp.report.record(ctx, kindMovepools, name, err)
	})
}

// keepImported removes from pool the moves imported rejects, and counts them (ADR-0012:
// no reference to a move missing from the catalog).
func keepImported(pool catalog.Movepool, imported func(id string) bool) (catalog.Movepool, int) {
	// pourquoi : make garantit une slice non nil, même vide : le handler refuse un
	// pourquoi : movepool null (« moves is required »), il accepte [].
	kept := make([]catalog.LearnedMove, 0, len(pool.Moves))
	for _, m := range pool.Moves {
		if imported(m.MoveID) {
			kept = append(kept, m)
		}
	}

	return catalog.Movepool{Moves: kept}, len(pool.Moves) - len(kept)
}

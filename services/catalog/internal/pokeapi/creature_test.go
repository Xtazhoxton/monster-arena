package pokeapi

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// Made-up version groups: order is chronological, unlike their position in a listing.
var testOrders = map[string]int{"vg-old": 1, "vg-mid": 2, "vg-new": 3}

func move(id string, details ...learnDetail) pokemonMove {
	return pokemonMove{Move: named{Name: id}, Details: details}
}

func learned(versionGroup, method string, level int) learnDetail {
	return learnDetail{Level: level, Method: named{Name: method}, VersionGroup: named{Name: versionGroup}}
}

func TestSelectMovepool(t *testing.T) {
	tests := []struct {
		name  string
		moves []pokemonMove
		want  []catalog.LearnedMove
	}{
		{
			name: "latest version group by order, not by position",
			moves: []pokemonMove{
				move("ember", learned("vg-mid", catalog.LevelUp, 7), learned("vg-old", catalog.LevelUp, 9)),
			},
			want: []catalog.LearnedMove{{MoveID: "ember", LearnMethod: catalog.LevelUp, Level: 7}},
		},
		{
			// vg-new mirrors PokéAPI's champions: recent, but without any level-up entry.
			name: "a version group without level-up is skipped",
			moves: []pokemonMove{
				move("ember", learned("vg-new", "train", 0), learned("vg-mid", catalog.LevelUp, 7)),
				move("surf", learned("vg-new", "train", 0)),
			},
			want: []catalog.LearnedMove{{MoveID: "ember", LearnMethod: catalog.LevelUp, Level: 7}},
		},
		{
			name: "moves missing from the chosen version group are dropped",
			moves: []pokemonMove{
				move("ember", learned("vg-mid", catalog.LevelUp, 7)),
				move("tackle", learned("vg-old", catalog.LevelUp, 1)),
			},
			want: []catalog.LearnedMove{{MoveID: "ember", LearnMethod: catalog.LevelUp, Level: 7}},
		},
		{
			name: "level-up beats machine, at its lowest level",
			moves: []pokemonMove{
				move("ember",
					learned("vg-mid", "machine", 0),
					learned("vg-mid", catalog.LevelUp, 20),
					learned("vg-mid", catalog.LevelUp, 7),
					learned("vg-mid", catalog.LevelUp, 12)),
			},
			want: []catalog.LearnedMove{{MoveID: "ember", LearnMethod: catalog.LevelUp, Level: 7}},
		},
		{
			name: "machine beats tutor beats egg beats the rest",
			moves: []pokemonMove{
				move("ember", learned("vg-mid", catalog.LevelUp, 7)),
				move("a", learned("vg-mid", "egg", 0), learned("vg-mid", "tutor", 0), learned("vg-mid", "machine", 0)),
				move("b", learned("vg-mid", "egg", 0), learned("vg-mid", "tutor", 0)),
				move("c", learned("vg-mid", "aaa-special", 0), learned("vg-mid", "egg", 0)),
			},
			want: []catalog.LearnedMove{
				{MoveID: "ember", LearnMethod: catalog.LevelUp, Level: 7},
				{MoveID: "a", LearnMethod: "machine"},
				{MoveID: "b", LearnMethod: "tutor"},
				{MoveID: "c", LearnMethod: "egg"},
			},
		},
		{
			name: "unknown methods are ordered alphabetically",
			moves: []pokemonMove{
				move("ember", learned("vg-mid", catalog.LevelUp, 7)),
				move("a", learned("vg-mid", "zz-method", 0), learned("vg-mid", "form-change", 0)),
			},
			want: []catalog.LearnedMove{
				{MoveID: "ember", LearnMethod: catalog.LevelUp, Level: 7},
				{MoveID: "a", LearnMethod: "form-change"},
			},
		},
		{
			name: "the level is dropped outside level-up",
			moves: []pokemonMove{
				move("ember", learned("vg-mid", catalog.LevelUp, 7)),
				move("surf", learned("vg-mid", "machine", 5)),
			},
			want: []catalog.LearnedMove{
				{MoveID: "ember", LearnMethod: catalog.LevelUp, Level: 7},
				{MoveID: "surf", LearnMethod: "machine"},
			},
		},
		{
			name:  "no level-up anywhere gives an empty movepool",
			moves: []pokemonMove{move("surf", learned("vg-mid", "machine", 0))},
			want:  []catalog.LearnedMove{},
		},
		{
			name:  "no moves gives an empty movepool",
			moves: nil,
			want:  []catalog.LearnedMove{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectMovepool(tt.moves, testOrders)

			// reflect.DeepEqual tells nil from empty: the handler refuses a nil movepool.
			if !reflect.DeepEqual(got.Moves, tt.want) {
				t.Errorf("selectMovepool() = %#v, want %#v", got.Moves, tt.want)
			}
			if err := got.Validate(); err != nil {
				t.Errorf("selectMovepool() is invalid: %v", err)
			}
		})
	}
}

func TestToCreatureUnknownGeneration(t *testing.T) {
	s := speciesResponse{Name: "flamkin", Generation: named{Name: "generation-x"}}

	if _, err := toCreature(s, pokemonResponse{}); err == nil {
		t.Error("toCreature() = nil error, want an error for an unknown generation")
	}
}

// writeCache stores body as the cached response for path, so the client never goes
// to the network.
func writeCache(t *testing.T, dir, path, body string) {
	t.Helper()

	file := filepath.Join(dir, url.QueryEscape(path)+".json")
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCreature decodes made-up PokéAPI documents through the cache: it checks the JSON
// tags and the choice of the default variety, whose name differs from the species.
func TestCreature(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	writeCache(t, dir, "pokemon-species/flamkin", `{
		"name": "flamkin",
		"names": [{"name": "Flamkine", "language": {"name": "fr"}}, {"name": "Flamkin", "language": {"name": "en"}}],
		"generation": {"name": "generation-iii"},
		"varieties": [
			{"is_default": false, "pokemon": {"name": "flamkin-blazing"}},
			{"is_default": true, "pokemon": {"name": "flamkin-normal"}}
		]
	}`)
	writeCache(t, dir, "pokemon/flamkin-normal", `{
		"stats": [
			{"base_stat": 39, "stat": {"name": "hp"}},
			{"base_stat": 52, "stat": {"name": "attack"}},
			{"base_stat": 43, "stat": {"name": "defense"}},
			{"base_stat": 60, "stat": {"name": "special-attack"}},
			{"base_stat": 50, "stat": {"name": "special-defense"}},
			{"base_stat": 65, "stat": {"name": "speed"}}
		],
		"types": [{"slot": 1, "type": {"name": "fire"}}, {"slot": 2, "type": {"name": "flying"}}],
		"moves": [{
			"move": {"name": "ember"},
			"version_group_details": [
				{"level_learned_at": 7, "move_learn_method": {"name": "level-up"}, "version_group": {"name": "vg-mid"}}
			]
		}]
	}`)

	creature, pool, err := c.Creature(context.Background(), "flamkin", testOrders)
	if err != nil {
		t.Fatalf("Creature() error = %v", err)
	}

	wantCreature := catalog.Creature{
		ID:         "flamkin",
		Name:       "Flamkin",
		Generation: 3,
		Types:      []string{"fire", "flying"},
		BaseStats:  catalog.Stats{HP: 39, Attack: 52, Defense: 43, SpecialAttack: 60, SpecialDefense: 50, Speed: 65},
	}
	if !reflect.DeepEqual(creature, wantCreature) {
		t.Errorf("Creature() creature = %+v, want %+v", creature, wantCreature)
	}

	wantMoves := []catalog.LearnedMove{{MoveID: "ember", LearnMethod: catalog.LevelUp, Level: 7}}
	if !reflect.DeepEqual(pool.Moves, wantMoves) {
		t.Errorf("Creature() movepool = %+v, want %+v", pool.Moves, wantMoves)
	}
}

func TestMoveNotForBattle(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeCache(t, dir, "move/dark-gust", `{
		"name": "dark-gust", "type": {"name": "shadow"}, "damage_class": {"name": "physical"},
		"power": 50, "accuracy": 100, "pp": null
	}`)

	if _, err := c.Move(context.Background(), "dark-gust"); !errors.Is(err, ErrNotForBattle) {
		t.Errorf("Move() error = %v, want ErrNotForBattle", err)
	}
}

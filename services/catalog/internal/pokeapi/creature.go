package pokeapi

import (
	"context"
	"fmt"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

var generations = map[string]int{
	"generation-i": 1, "generation-ii": 2, "generation-iii": 3,
	"generation-iv": 4, "generation-v": 5, "generation-vi": 6,
	"generation-vii": 7, "generation-viii": 8, "generation-ix": 9,
}

// methodRank orders the learn methods a move may be listed under, best first (ADR-0012).
var methodRank = map[string]int{catalog.LevelUp: 0, "machine": 1, "tutor": 2, "egg": 3}

// rank returns the rank of method; the methods not in methodRank share the last one.
func rank(method string) int {
	// syntaxe : « comma ok » : ok dit si la clé existe. Sans lui, une méthode inconnue
	// syntaxe : rendrait 0, c'est-à-dire le rang de level-up : la meilleure !
	if r, ok := methodRank[method]; ok {
		return r
	}

	return len(methodRank)
}

// better reports whether a is preferred over b, two entries of the same move.
func better(a, b catalog.LearnedMove) bool {
	// syntaxe : un if peut déclarer ses variables avant le « ; » : elles n'existent que
	// syntaxe : dans ce if. Ici, deux d'un coup.
	if ra, rb := rank(a.LearnMethod), rank(b.LearnMethod); ra != rb {
		return ra < rb
	}
	// pourquoi : même rang mais méthodes différentes = deux méthodes « autres » :
	// pourquoi : l'ordre alphabétique départage, pour que le résultat soit déterministe.
	if a.LearnMethod != b.LearnMethod {
		return a.LearnMethod < b.LearnMethod
	}

	return a.Level < b.Level
}

// selectMovepool applies the ADR-0012 rule: keep the latest version group that teaches
// at least one move by level-up, then the best entry of each move within it.
func selectMovepool(moves []pokemonMove, orders map[string]int) catalog.Movepool {
	// pourquoi : passe 1 : trouver le version group. On compare les order, pas les noms :
	// pourquoi : la position dans la liste de PokéAPI n'est pas chronologique.
	latest := ""
	for _, m := range moves {
		for _, d := range m.Details {
			if d.Method.Name == catalog.LevelUp && (latest == "" || orders[d.VersionGroup.Name] > orders[latest]) {
				latest = d.VersionGroup.Name
			}
		}
	}

	// pourquoi : une slice vide et non nil : encodée en JSON, nil donne null, que notre
	// pourquoi : handler refuse (« moves is required »). [] veut dire « movepool vide ».
	pool := catalog.Movepool{Moves: []catalog.LearnedMove{}}
	if latest == "" {
		return pool
	}

	// pourquoi : passe 2 : pour chaque attaque, la meilleure entrée de ce version group.
	for _, m := range moves {
		var best catalog.LearnedMove
		for _, d := range m.Details {
			if d.VersionGroup.Name != latest {
				continue
			}

			candidate := catalog.LearnedMove{MoveID: m.Move.Name, LearnMethod: d.Method.Name}
			// pourquoi : Validate refuse un niveau hors level-up : on ne recopie le
			// pourquoi : niveau de PokéAPI que là où il a un sens.
			if candidate.LearnMethod == catalog.LevelUp {
				candidate.Level = d.Level
			}

			// pourquoi : best.MoveID vide = valeur zéro = « pas encore de candidat ».
			if best.MoveID == "" || better(candidate, best) {
				best = candidate
			}
		}

		// pourquoi : une attaque absente de ce version group n'entre pas au movepool.
		if best.MoveID != "" {
			pool.Moves = append(pool.Moves, best)
		}
	}

	return pool
}

// toCreature builds a creature from its species and its default variety.
func toCreature(s speciesResponse, p pokemonResponse) (catalog.Creature, error) {
	generation, ok := generations[s.Generation.Name]
	if !ok {
		// pourquoi : une erreur plutôt que 0 : la génération 10 arrivera un jour, et un
		// pourquoi : 0 silencieux finirait en base. L'erreur nous dira d'ajouter la ligne.
		return catalog.Creature{}, fmt.Errorf("species %s: unknown generation %q", s.Name, s.Generation.Name)
	}

	creature := catalog.Creature{
		ID:         s.Name,
		Name:       englishName(s.Names, s.Name),
		Generation: generation,
	}
	for _, t := range p.Types {
		creature.Types = append(creature.Types, t.Type.Name)
	}
	for _, st := range p.Stats {
		switch st.Stat.Name {
		case "hp":
			creature.BaseStats.HP = st.BaseStat
		case "attack":
			creature.BaseStats.Attack = st.BaseStat
		case "defense":
			creature.BaseStats.Defense = st.BaseStat
		case "special-attack":
			creature.BaseStats.SpecialAttack = st.BaseStat
		case "special-defense":
			creature.BaseStats.SpecialDefense = st.BaseStat
		case "speed":
			creature.BaseStats.Speed = st.BaseStat
		}
	}

	return creature, nil
}

// Creature returns the species in its default form, with its movepool.
func (c *Client) Creature(ctx context.Context, species string, orders map[string]int) (catalog.Creature, catalog.Movepool, error) {
	var s speciesResponse
	if err := c.get(ctx, "pokemon-species/"+species, &s); err != nil {
		return catalog.Creature{}, catalog.Movepool{}, err
	}

	variety := ""
	for _, v := range s.Varieties {
		if v.IsDefault {
			variety = v.Pokemon.Name
			break
		}
	}
	if variety == "" {
		return catalog.Creature{}, catalog.Movepool{}, fmt.Errorf("species %s: no default variety", species)
	}

	var p pokemonResponse
	if err := c.get(ctx, "pokemon/"+variety, &p); err != nil {
		return catalog.Creature{}, catalog.Movepool{}, err
	}

	creature, err := toCreature(s, p)
	if err != nil {
		return catalog.Creature{}, catalog.Movepool{}, err
	}

	// pourquoi : l'id est le nom de l'espèce (deoxys), pas celui de la forme
	// pourquoi : (deoxys-normal) : toCreature prend s.Name.
	return creature, selectMovepool(p.Moves, orders), nil
}

package pokeapi

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// nonBattleTypes are PokéAPI types no creature fights with (ADR-0012).
var nonBattleTypes = map[string]bool{"stellar": true, "unknown": true, "shadow": true}

// ErrNotForBattle marks a PokéAPI document no creature fights with, such as the shadow
// moves of Colosseum: the import skips it instead of counting it as refused.
var ErrNotForBattle = errors.New("not used in battle")

// List returns the names of every document of resource ("move", "pokemon-species"…).
func (c *Client) List(ctx context.Context, resource string) ([]string, error) {
	var resp listResponse
	// pourquoi : une limite plus grande que tout listing PokéAPI (1025 espèces) : tout
	// pourquoi : arrive en une page, sans gérer la pagination.
	if err := c.get(ctx, resource+"?limit=10000", &resp); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(resp.Results))
	for _, r := range resp.Results {
		names = append(names, r.Name)
	}

	return names, nil
}

// BattleTypeNames returns the names of the 18 types creatures fight with.
func (c *Client) BattleTypeNames(ctx context.Context) ([]string, error) {
	names, err := c.List(ctx, "type")
	if err != nil {
		return nil, err
	}

	// syntaxe : DeleteFunc retire de la slice les éléments pour lesquels la fonction rend
	// syntaxe : true. La fonction est anonyme, comme celle du defer de download.
	return slices.DeleteFunc(names, func(name string) bool { return nonBattleTypes[name] }), nil
}

// englishName returns the English display name, or fallback when there is none.
func englishName(names []localizedName, fallback string) string {
	for _, n := range names {
		if n.Language.Name == "en" {
			return n.Name
		}
	}

	return fallback
}

// Type returns the type name with its row of the type chart.
func (c *Client) Type(ctx context.Context, name string) (catalog.Type, error) {
	var resp typeResponse
	if err := c.get(ctx, "type/"+name, &resp); err != nil {
		return catalog.Type{}, err
	}

	effectiveness := make(map[string]float64)

	// pourquoi : trois listes, même traitement : une fonction anonyme rangée dans une
	// pourquoi : variable évite de recopier la boucle trois fois. Elle « voit » et modifie
	// pourquoi : effectiveness, déclarée au-dessus : c'est ce qu'on appelle une closure.
	record := func(defenders []named, multiplier float64) {
		for _, d := range defenders {
			if !nonBattleTypes[d.Name] {
				effectiveness[d.Name] = multiplier
			}
		}
	}

	// pourquoi : rien pour le 1 : PokéAPI ne liste que les écarts, notre matrice est creuse
	// pourquoi : de la même façon (absent = 1). Les deux formats coïncident.
	record(resp.DamageRelations.DoubleDamageTo, 2)
	record(resp.DamageRelations.HalfDamageTo, 0.5)
	record(resp.DamageRelations.NoDamageTo, 0)

	return catalog.Type{
		ID:            resp.Name,
		Name:          englishName(resp.Names, resp.Name),
		Effectiveness: effectiveness,
	}, nil
}

// Move returns the move name, effects aside (deferred to the battle engine).
func (c *Client) Move(ctx context.Context, name string) (catalog.Move, error) {
	var resp moveResponse
	if err := c.get(ctx, "move/"+name, &resp); err != nil {
		return catalog.Move{}, err
	}
	// pourquoi : même règle que pour les types : une attaque d'un type hors combat ne
	// pourquoi : pourrait viser aucune créature.
	if nonBattleTypes[resp.Type.Name] {
		return catalog.Move{}, fmt.Errorf("move %s of type %s: %w", name, resp.Type.Name, ErrNotForBattle)
	}

	// pourquoi : aucune validation ici : c'est le travail de notre API. Une attaque que
	// pourquoi : PokéAPI décrit mal sortira en 400, journalisée par l'import (ADR-0012).
	return catalog.Move{
		ID:          resp.Name,
		Name:        englishName(resp.Names, resp.Name),
		Type:        resp.Type.Name,
		DamageClass: catalog.DamageClass(resp.DamageClass.Name),
		Power:       resp.Power,
		Accuracy:    resp.Accuracy,
		PP:          resp.PP,
	}, nil
}

// VersionGroupOrders maps every version group to its chronological order, on which
// the movepool selection relies: the listing's own order is not chronological.
func (c *Client) VersionGroupOrders(ctx context.Context) (map[string]int, error) {
	names, err := c.List(ctx, "version-group")
	if err != nil {
		return nil, err
	}

	orders := make(map[string]int, len(names))
	for _, name := range names {
		var resp versionGroupResponse
		if err := c.get(ctx, "version-group/"+name, &resp); err != nil {
			return nil, err
		}
		orders[resp.Name] = resp.Order
	}

	return orders, nil
}

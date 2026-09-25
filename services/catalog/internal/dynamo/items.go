package dynamo

import (
	"fmt"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// statsItem mirrors catalog.Stats with DynamoDB attribute names.
type statsItem struct {
	HP             int `dynamodbav:"hp"`
	Attack         int `dynamodbav:"attack"`
	Defense        int `dynamodbav:"defense"`
	SpecialAttack  int `dynamodbav:"specialAttack"`
	SpecialDefense int `dynamodbav:"specialDefense"`
	Speed          int `dynamodbav:"speed"`
}

// creatureItem is the profile item: PK = CREATURE#<id>, SK = CREATURE.
// Types are denormalised here so that listing creatures needs a single query (ADR-0010).
type creatureItem struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	Name       string    `dynamodbav:"name"`
	Generation int       `dynamodbav:"generation"`
	Types      []string  `dynamodbav:"types"`
	BaseStats  statsItem `dynamodbav:"baseStats"`
}

// creatureTypeItem links a creature to one of its types: SK = TYPE#<id>.
type creatureTypeItem struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
}

// creatureMoveItem is one movepool entry: SK = MOVE#<id>.
type creatureMoveItem struct {
	PK          string `dynamodbav:"PK"`
	SK          string `dynamodbav:"SK"`
	LearnMethod string `dynamodbav:"learnMethod"`
	Level       int    `dynamodbav:"level,omitempty"`
}

// newCreatureItem builds the profile item of c.
func newCreatureItem(c catalog.Creature) creatureItem {
	return creatureItem{
		PK:         creaturePK(c.ID),
		SK:         creatureEntity,
		Name:       c.Name,
		Generation: c.Generation,
		Types:      c.Types,
		BaseStats:  statsItem(c.BaseStats),
	}
}

// creature rebuilds the domain value, without its types' details or its movepool.
func (i creatureItem) creature() (catalog.Creature, error) {
	id, ok := idAfter(i.PK, creatureEntity)
	if !ok {
		return catalog.Creature{}, fmt.Errorf("malformed creature item: PK %q", i.PK)
	}

	return catalog.Creature{
		ID:         id,
		Name:       i.Name,
		Generation: i.Generation,
		Types:      i.Types,
		BaseStats:  catalog.Stats(i.BaseStats),
	}, nil
}

// newCreatureTypeItems builds the link items for the types of c, in declaration order.
func newCreatureTypeItems(c catalog.Creature) []creatureTypeItem {
	items := make([]creatureTypeItem, 0, len(c.Types))
	for _, typeID := range c.Types {
		items = append(items, creatureTypeItem{
			PK: creaturePK(c.ID),
			SK: creatureTypeSK(typeID),
		})
	}

	return items
}

// newCreatureMoveItem builds the movepool item of m for the creature id.
func newCreatureMoveItem(creatureID string, m catalog.LearnedMove) creatureMoveItem {
	return creatureMoveItem{
		PK:          creaturePK(creatureID),
		SK:          creatureMoveSK(m.MoveID),
		LearnMethod: m.LearnMethod,
		Level:       m.Level,
	}
}

// learnedMove rebuilds the domain value from a movepool item.
func (i creatureMoveItem) learnedMove() (catalog.LearnedMove, error) {
	moveID, ok := idAfter(i.SK, moveEntity)
	if !ok {
		return catalog.LearnedMove{}, fmt.Errorf("malformed movepool item: SK %q", i.SK)
	}

	return catalog.LearnedMove{
		MoveID:      moveID,
		LearnMethod: i.LearnMethod,
		Level:       i.Level,
	}, nil
}

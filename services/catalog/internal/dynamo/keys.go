// Package dynamo stores the catalog in a single DynamoDB table (see ADR-0010).
package dynamo

import "strings"

// Key components. The separator is safe because Validate rejects any identifier
// containing it: no user input can forge a key.
const (
	sep            = "#"
	creatureEntity = "CREATURE"
	moveEntity     = "MOVE"
	typeEntity     = "TYPE"
	versusEntity   = "VS"
	indexInverted  = "GSI1"
)

// creaturePK returns the partition holding a creature and everything attached to it.
func creaturePK(id string) string { return creatureEntity + sep + id }

// movePK returns the partition holding a move.
func movePK(id string) string { return moveEntity + sep + id }

// typePK returns the partition holding a type and its effectiveness row.
func typePK(id string) string { return typeEntity + sep + id }

// creatureTypeSK returns the sort key of one type of a creature.
func creatureTypeSK(typeID string) string { return typeEntity + sep + typeID }

// creatureMoveSK returns the sort key of one movepool entry.
func creatureMoveSK(moveID string) string { return moveEntity + sep + moveID }

// versusSK returns the sort key of one cell of the effectiveness matrix.
func versusSK(defenderID string) string { return versusEntity + sep + defenderID }

// idAfter returns the identifier following entity+sep in key, and whether key had that shape.
func idAfter(key, entity string) (string, bool) {
	id, found := strings.CutPrefix(key, entity+sep)
	if !found || id == "" {
		return "", false
	}
	return id, true
}

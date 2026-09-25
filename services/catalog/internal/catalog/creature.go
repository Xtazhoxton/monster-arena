// Package catalog contains the domain model of the creature catalog:
// creatures, moves and types, independent of storage and transport.
package catalog

import (
	"errors"
	"fmt"
)

// Stats holds the six base statistics of a creature.
type Stats struct {
	HP             int `json:"hp"`
	Attack         int `json:"attack"`
	Defense        int `json:"defense"`
	SpecialAttack  int `json:"specialAttack"`
	SpecialDefense int `json:"specialDefense"`
	Speed          int `json:"speed"`
}

// LearnedMove is one entry of a creature's movepool.
type LearnedMove struct {
	MoveID      string `json:"id"`
	LearnMethod string `json:"learnMethod"`
	Level       int    `json:"level,omitempty"`
}

// Creature is a species: its identity, its types and its base statistics.
type Creature struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Generation int           `json:"generation"`
	Types      []string      `json:"types"`
	BaseStats  Stats         `json:"baseStats"`
	Moves      []LearnedMove `json:"moves,omitempty"`
}

// CreaturePage is one page of a creature listing: the creatures themselves,
// and the cursor to pass back to obtain the next page.
// An empty NextCursor means the last page was reached.
type CreaturePage struct {
	Creatures  []Creature `json:"creatures"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// Validate reports every reason the creature cannot be stored, or nil if it can.
func (c Creature) Validate() error {
	var errs []error

	if !IsSlug(c.ID) {
		errs = append(errs, fmt.Errorf("%w: id %q must be a slug", ErrInvalid, c.ID))
	}
	if c.Name == "" {
		errs = append(errs, fmt.Errorf("%w: name is empty", ErrInvalid))
	}
	if len(c.Types) == 0 || len(c.Types) > 2 {
		errs = append(errs, fmt.Errorf("%w: want 1 or 2 types, got %d", ErrInvalid, len(c.Types)))
	}
	if len(c.Types) == 2 && c.Types[0] == c.Types[1] {
		errs = append(errs, fmt.Errorf("%w: duplicate type %q", ErrInvalid, c.Types[0]))
	}
	for _, t := range c.Types {
		if !IsSlug(t) {
			errs = append(errs, fmt.Errorf("%w: type %q must be a slug", ErrInvalid, t))
		}
	}
	for _, m := range c.Moves {
		if !IsSlug(m.MoveID) {
			errs = append(errs, fmt.Errorf("%w: move %q must be a slug", ErrInvalid, m.MoveID))
		}
	}

	return errors.Join(errs...)
}

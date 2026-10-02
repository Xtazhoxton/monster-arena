package catalog

import (
	"errors"
	"fmt"
)

// LevelUp is the learn method whose entries may carry a level.
const LevelUp = "level-up"

// Movepool is the whole set of moves a creature can learn. It is written in one piece
// through PUT /creatures/{id}/moves: a move absent from it is forgotten.
type Movepool struct {
	Moves []LearnedMove `json:"moves"`
}

// Validate reports every reason the movepool cannot be stored, or nil if it can.
func (p Movepool) Validate() error {
	var errs []error

	seen := make(map[string]bool, len(p.Moves))

	for _, m := range p.Moves {
		if !IsSlug(m.MoveID) {
			errs = append(errs, fmt.Errorf("%w: move %q must be a slug", ErrInvalid, m.MoveID))
		}
		if seen[m.MoveID] {
			errs = append(errs, fmt.Errorf("%w: move %q is listed twice", ErrInvalid, m.MoveID))
		}
		seen[m.MoveID] = true

		if !IsSlug(m.LearnMethod) {
			errs = append(errs, fmt.Errorf("%w: learn method %q of move %q must be a slug", ErrInvalid, m.LearnMethod, m.MoveID))
		}
		if m.Level < 0 {
			errs = append(errs, fmt.Errorf("%w: move %q has negative level %d", ErrInvalid, m.MoveID, m.Level))
		}
		if m.Level > 0 && m.LearnMethod != LevelUp {
			errs = append(errs, fmt.Errorf("%w: move %q: only %q entries have a level", ErrInvalid, m.MoveID, LevelUp))
		}
	}
	return errors.Join(errs...)
}

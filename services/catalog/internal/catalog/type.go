package catalog

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

// Type is an element a creature or a move belongs to, together with the row of the
// type chart it attacks with: Effectiveness maps a defending type id to the damage
// multiplier. A defending type ABSENT from the map takes normal damage, that is 1.
type Type struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Effectiveness map[string]float64 `json:"effectiveness"`
}

// TypePage is one page of a type listing, with the cursor for the next one.
// An empty NextCursor means the last page was reached.
type TypePage struct {
	Types      []Type `json:"types"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// validMultiplier reports whether m is a multiplier the type chart may hold.
// 1 is deliberately absent: it is the default, and accepting it would give the
// same chart two different stored forms.
func validMultiplier(m float64) bool {
	switch m {
	case 0, 0.5, 2:
		return true

	default:
		return false
	}
}

// Validate reports every reason the type cannot be stored, or nil if it can.
func (t Type) Validate() error {
	var errs []error

	if !IsSlug(t.ID) {
		errs = append(errs, fmt.Errorf("%w: id %q must be a slug", ErrInvalid, t.ID))
	}
	if t.Name == "" {
		errs = append(errs, fmt.Errorf("%w: name is empty", ErrInvalid))
	}

	for _, defender := range slices.Sorted(maps.Keys(t.Effectiveness)) {
		multiplier := t.Effectiveness[defender]

		if !IsSlug(defender) {
			errs = append(errs, fmt.Errorf("%w: defending type %q must be a slug", ErrInvalid, defender))
		}

		if multiplier == 1 {
			errs = append(errs, fmt.Errorf("%w: multiplier against %q is 1, the default: omit it", ErrInvalid, defender))
			continue
		}

		if !validMultiplier(multiplier) {
			errs = append(errs, fmt.Errorf("%w: multiplier %v against %q must be 0, 0.5 or 2", ErrInvalid, multiplier, defender))
		}
	}

	return errors.Join(errs...)
}

// Against returns the damage multiplier of t against the defending type id.
// A defender the chart does not mention takes normal damage.
func (t Type) Against(defenderID string) float64 {
	if multiplier, ok := t.Effectiveness[defenderID]; ok {
		return multiplier
	}

	return 1
}

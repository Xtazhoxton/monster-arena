package catalog

import (
	"errors"
	"fmt"
)

// DamageClass tells how a move computes its damage.
type DamageClass string

// The three damage classes. A status move deals no damage at all.
const (
	Physical DamageClass = "physical"
	Special  DamageClass = "special"
	Status   DamageClass = "status"
)

// valid reports whether d is one of the three known damage classes.
func (d DamageClass) valid() bool {
	switch d {
	case Physical, Special, Status:
		return true

	default:
		return false
	}
}

// Move is an attack a creature can learn: what the move does, not who knows it.
// Power is 0 for a move that deals no direct damage, and Accuracy is 0 for a
// move that cannot miss.
type Move struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	DamageClass DamageClass `json:"damageClass"`
	Power       int         `json:"power"`
	Accuracy    int         `json:"accuracy"`
	PP          int         `json:"pp"`
}

// MovePage is one page of a move listing, with the cursor for the next one.
// An empty NextCursor means the last page was reached.
type MovePage struct {
	Moves      []Move `json:"moves"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Validate reports every reason the move cannot be stored, or nil if it can.
func (m Move) Validate() error {
	var errs []error

	if !IsSlug(m.ID) {
		errs = append(errs, fmt.Errorf("%w: id %q must be a slug", ErrInvalid, m.ID))
	}
	if m.Name == "" {
		errs = append(errs, fmt.Errorf("%w: name is empty", ErrInvalid))
	}
	if !IsSlug(m.Type) {
		errs = append(errs, fmt.Errorf("%w: type %q must be a slug", ErrInvalid, m.Type))
	}
	if !m.DamageClass.valid() {
		errs = append(errs, fmt.Errorf("%w: unknown damage class %q", ErrInvalid, m.DamageClass))
	}
	if m.Power < 0 {
		errs = append(errs, fmt.Errorf("%w: power %d is negative", ErrInvalid, m.Power))
	}
	if m.DamageClass == Status && m.Power != 0 {
		errs = append(errs, fmt.Errorf("%w: status move cannot have power %d", ErrInvalid, m.Power))
	}
	if m.Accuracy < 0 || m.Accuracy > 100 {
		errs = append(errs, fmt.Errorf("%w: accuracy %d is out of [0, 100]", ErrInvalid, m.Accuracy))
	}
	if m.PP <= 0 {
		errs = append(errs, fmt.Errorf("%w: pp %d must be positive", ErrInvalid, m.PP))
	}

	return errors.Join(errs...)
}

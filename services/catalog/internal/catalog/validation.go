package catalog

import (
	"errors"
	"regexp"
)

// ErrInvalid is wrapped by every validation error, so callers can map it to HTTP 400.
var ErrInvalid = errors.New("invalid")

// slugPattern matches the identifiers used as key suffixes: "pikachu", "thunder-punch".
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// isSlug reports whether s is safe to embed in a partition or sort key.
func isSlug(s string) bool {
	return slugPattern.MatchString(s)
}

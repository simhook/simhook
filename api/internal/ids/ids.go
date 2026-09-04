// Package ids generates time-ordered identifiers.
package ids

import "github.com/google/uuid"

// New returns a UUIDv7. Time-ordered ids keep b-tree inserts sequential and
// make "newest first" a plain index scan.
func New() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		// NewV7 only fails when the random source does, which is fatal anyway.
		panic(err)
	}
	return id
}

// Parse accepts any textual UUID form and reports whether it is valid.
func Parse(s string) (uuid.UUID, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

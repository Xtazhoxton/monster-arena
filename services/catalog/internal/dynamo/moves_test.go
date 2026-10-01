package dynamo

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// aMove returns the move the tests below store and read back.
func aMove() catalog.Move {
	return catalog.Move{
		ID:          "thunderbolt",
		Name:        "Thunderbolt",
		Type:        "electric",
		DamageClass: catalog.Special,
		Power:       90,
		Accuracy:    100,
		PP:          15,
	}
}

// TestMoveItemRoundTrip pins the keys of a move item and checks that the domain
// value survives the trip through DynamoDB attribute names.
func TestMoveItemRoundTrip(t *testing.T) {
	want := aMove()

	item := newMoveItem(want)
	if item.PK != "MOVE#thunderbolt" || item.SK != "MOVE" {
		t.Fatalf("keys = (%q, %q), want (%q, %q)", item.PK, item.SK, "MOVE#thunderbolt", "MOVE")
	}

	got, err := item.move()
	if err != nil {
		t.Fatalf("move() = %v, want nil", err)
	}

	if got != want {
		t.Errorf("round trip gave %+v, want %+v", got, want)
	}
}

// TestMoveItemAttributeNames pins the names actually written to DynamoDB.
// The round trip above cannot catch a wrong dynamodbav tag: the same struct is
// used to write and to read, so a typo stays invisible until another reader —
// a query projection, an export, the battle engine — looks for the real name.
func TestMoveItemAttributeNames(t *testing.T) {
	attrs, err := attributevalue.MarshalMap(newMoveItem(aMove()))
	if err != nil {
		t.Fatalf("MarshalMap() = %v, want nil", err)
	}

	want := []string{"PK", "SK", "accuracy", "damageClass", "name", "power", "pp", "type"}

	got := make([]string, 0, len(attrs))
	for name := range attrs {
		got = append(got, name)
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("attributes = %v, want %v", got, want)
	}
}

func TestMoveItemMalformedPK(t *testing.T) {
	tests := []struct {
		name string
		pk   string
	}{
		{name: "empty", pk: ""},
		{name: "no prefix", pk: "thunderbolt"},
		{name: "wrong entity", pk: "CREATURE#pikachu"},
		{name: "prefix without id", pk: "MOVE#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := newMoveItem(aMove())
			item.PK = tt.pk

			if _, err := item.move(); err == nil {
				t.Errorf("move() on PK %q = nil, want an error", tt.pk)
			}
		})
	}
}

// TestPutMoveRejectsInvalidMove checks that PutMove validates before it writes.
// The store carries a nil client on purpose: reaching DynamoDB would panic, so
// "the invalid move never left the process" is asserted for free.
func TestPutMoveRejectsInvalidMove(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*catalog.Move)
	}{
		{name: "key injection in id", mutate: func(m *catalog.Move) { m.ID = "thunderbolt#SK#x" }},
		{name: "empty id", mutate: func(m *catalog.Move) { m.ID = "" }},
		{name: "unknown damage class", mutate: func(m *catalog.Move) { m.DamageClass = "magical" }},
		{name: "zero pp", mutate: func(m *catalog.Move) { m.PP = 0 }},
	}

	store := New(nil, "monster-arena-test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := aMove()
			tt.mutate(&m)

			err := store.PutMove(context.Background(), m)
			if !errors.Is(err, catalog.ErrInvalid) {
				t.Errorf("PutMove() = %v, want an error wrapping ErrInvalid", err)
			}
		})
	}
}

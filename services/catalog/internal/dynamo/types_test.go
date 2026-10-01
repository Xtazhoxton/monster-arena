package dynamo

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// aType returns the type the tests below store and read back.
func aType() catalog.Type {
	return catalog.Type{
		ID:   "fire",
		Name: "Fire",
		Effectiveness: map[string]float64{
			"grass": 2,
			"water": 0.5,
			"ghost": 0,
		},
	}
}

// TestTypeItemRoundTrip pins the keys of a type item and checks that the domain value
// survives the trip, multipliers included: 0, 0.5 and 2 must come back untouched.
func TestTypeItemRoundTrip(t *testing.T) {
	want := aType()

	item := newTypeItem(want)
	if item.PK != "TYPE#fire" || item.SK != "TYPE" {
		t.Fatalf("keys = (%q, %q), want (%q, %q)", item.PK, item.SK, "TYPE#fire", "TYPE")
	}

	got, err := item.elementalType()
	if err != nil {
		t.Fatalf("elementalType() = %v, want nil", err)
	}

	// catalog.Type holds a map, so it is not comparable with ==.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip gave %+v, want %+v", got, want)
	}
}

// TestTypeItemRoundTripThroughAttributeValues goes through the SDK as well, because the
// conversion above never touches the map: only marshalling can lose a multiplier.
func TestTypeItemRoundTripThroughAttributeValues(t *testing.T) {
	want := aType()

	attrs, err := attributevalue.MarshalMap(newTypeItem(want))
	if err != nil {
		t.Fatalf("MarshalMap() = %v, want nil", err)
	}

	var item typeItem
	if err := attributevalue.UnmarshalMap(attrs, &item); err != nil {
		t.Fatalf("UnmarshalMap() = %v, want nil", err)
	}

	got, err := item.elementalType()
	if err != nil {
		t.Fatalf("elementalType() = %v, want nil", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip gave %+v, want %+v", got, want)
	}
}

// TestTypeItemAttributeNames pins the names written to DynamoDB, and checks that
// omitempty really keeps an empty chart out of the table instead of writing NULL.
func TestTypeItemAttributeNames(t *testing.T) {
	tests := []struct {
		name  string
		chart map[string]float64
		want  []string
	}{
		{name: "with a chart", chart: aType().Effectiveness, want: []string{"PK", "SK", "effectiveness", "name"}},
		{name: "nil chart is omitted", chart: nil, want: []string{"PK", "SK", "name"}},
		{name: "empty chart is omitted", chart: map[string]float64{}, want: []string{"PK", "SK", "name"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ty := aType()
			ty.Effectiveness = tt.chart

			attrs, err := attributevalue.MarshalMap(newTypeItem(ty))
			if err != nil {
				t.Fatalf("MarshalMap() = %v, want nil", err)
			}

			got := make([]string, 0, len(attrs))
			for name := range attrs {
				got = append(got, name)
			}
			slices.Sort(got)

			if !slices.Equal(got, tt.want) {
				t.Errorf("attributes = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTypeItemMissingChartReadsAsNormalDamage is the reason omitempty is safe: a type
// stored without a chart must behave as "everything takes normal damage", not as
// "everything is immune".
func TestTypeItemMissingChartReadsAsNormalDamage(t *testing.T) {
	ty := aType()
	ty.Effectiveness = nil

	attrs, err := attributevalue.MarshalMap(newTypeItem(ty))
	if err != nil {
		t.Fatalf("MarshalMap() = %v, want nil", err)
	}

	var item typeItem
	if err := attributevalue.UnmarshalMap(attrs, &item); err != nil {
		t.Fatalf("UnmarshalMap() = %v, want nil", err)
	}

	got, err := item.elementalType()
	if err != nil {
		t.Fatalf("elementalType() = %v, want nil", err)
	}

	if multiplier := got.Against("grass"); multiplier != 1 {
		t.Errorf("Against(%q) = %v on a chartless type, want 1", "grass", multiplier)
	}
}

func TestTypeItemMalformedPK(t *testing.T) {
	tests := []struct {
		name string
		pk   string
	}{
		{name: "empty", pk: ""},
		{name: "no prefix", pk: "fire"},
		{name: "wrong entity", pk: "MOVE#thunderbolt"},
		{name: "prefix without id", pk: "TYPE#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := newTypeItem(aType())
			item.PK = tt.pk

			if _, err := item.elementalType(); err == nil {
				t.Errorf("elementalType() on PK %q = nil, want an error", tt.pk)
			}
		})
	}
}

// TestPutTypeRejectsInvalidType checks that PutType validates before it writes.
// The store carries a nil client on purpose: reaching DynamoDB would panic.
func TestPutTypeRejectsInvalidType(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*catalog.Type)
	}{
		{name: "key injection in id", mutate: func(ty *catalog.Type) { ty.ID = "fire#SK#x" }},
		{name: "empty id", mutate: func(ty *catalog.Type) { ty.ID = "" }},
		{name: "defender is not a slug", mutate: func(ty *catalog.Type) { ty.Effectiveness["Sound Wave"] = 2 }},
		{name: "explicit default multiplier", mutate: func(ty *catalog.Type) { ty.Effectiveness["normal"] = 1 }},
		{name: "multiplier out of the chart", mutate: func(ty *catalog.Type) { ty.Effectiveness["rock"] = 3 }},
	}

	store := New(nil, "monster-arena-test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ty := aType()
			tt.mutate(&ty)

			err := store.PutType(context.Background(), ty)
			if !errors.Is(err, catalog.ErrInvalid) {
				t.Errorf("PutType() = %v, want an error wrapping ErrInvalid", err)
			}
		})
	}
}

package catalog

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// validType returns the type every Validate case starts from.
func validType() Type {
	return Type{
		ID:   "fire",
		Name: "Fire",
		Effectiveness: map[string]float64{
			"grass": 2,
			"water": 0.5,
			"fire":  0.5,
		},
	}
}

func TestTypeValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Type)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Type) {}},
		{name: "no chart at all", mutate: func(ty *Type) { ty.Effectiveness = nil }},
		{name: "empty chart", mutate: func(ty *Type) { ty.Effectiveness = map[string]float64{} }},
		{name: "immunity", mutate: func(ty *Type) { ty.Effectiveness["ghost"] = 0 }},
		{name: "a type may resist itself", mutate: func(ty *Type) { ty.Effectiveness["fire"] = 0.5 }},

		{name: "empty id", mutate: func(ty *Type) { ty.ID = "" }, wantErr: true},
		{name: "key injection in id", mutate: func(ty *Type) { ty.ID = "fire#VS#water" }, wantErr: true},
		{name: "uppercase id", mutate: func(ty *Type) { ty.ID = "Fire" }, wantErr: true},
		{name: "empty name", mutate: func(ty *Type) { ty.Name = "" }, wantErr: true},
		{name: "defender is not a slug", mutate: func(ty *Type) { ty.Effectiveness["Sound Wave"] = 2 }, wantErr: true},
		{name: "explicit 1 is refused", mutate: func(ty *Type) { ty.Effectiveness["normal"] = 1 }, wantErr: true},
		{name: "multiplier out of the chart", mutate: func(ty *Type) { ty.Effectiveness["rock"] = 3 }, wantErr: true},
		{name: "quarter multiplier", mutate: func(ty *Type) { ty.Effectiveness["rock"] = 0.25 }, wantErr: true},
		{name: "negative multiplier", mutate: func(ty *Type) { ty.Effectiveness["rock"] = -2 }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ty := validType()
			tt.mutate(&ty)

			err := ty.Validate()
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Errorf("Validate() = %v, want an error wrapping ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestTypeValidateExplicitOneSaysOnlyOmitIt checks that a multiplier of 1 is reported
// once, with the actionable message, and not also as an out-of-chart value.
func TestTypeValidateExplicitOneSaysOnlyOmitIt(t *testing.T) {
	ty := validType()
	ty.Effectiveness = map[string]float64{"normal": 1}

	err := ty.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error")
	}

	got := err.Error()
	if !strings.Contains(got, "omit it") {
		t.Errorf("Validate() = %q, want it to mention omitting the default", got)
	}
	if strings.Contains(got, "must be 0, 0.5 or 2") {
		t.Errorf("Validate() = %q, want a single message, not also the out-of-chart one", got)
	}
}

// TestTypeValidateOrdersItsProblems checks that the report does not depend on Go's
// randomised map iteration: the same chart must always produce the same message.
func TestTypeValidateOrdersItsProblems(t *testing.T) {
	ty := Type{
		ID:            "fire",
		Name:          "Fire",
		Effectiveness: map[string]float64{"aaa": 3, "bbb": 3, "ccc": 3, "ddd": 3, "eee": 3},
	}

	first := ty.Validate().Error()
	for range 50 {
		if got := ty.Validate().Error(); got != first {
			t.Fatalf("two runs disagree:\n%s\n%s", first, got)
		}
	}

	if want := "aaa"; !strings.Contains(strings.SplitN(first, "\n", 2)[0], want) {
		t.Errorf("first reported problem = %q, want the one about %q", first, want)
	}
}

// TestTypeAgainst is the reason Against exists: reading the map directly returns 0
// for an unknown defender, which means immunity — the opposite of the convention.
func TestTypeAgainst(t *testing.T) {
	tests := []struct {
		name     string
		chart    map[string]float64
		defender string
		want     float64
	}{
		{name: "listed as weak", chart: map[string]float64{"grass": 2}, defender: "grass", want: 2},
		{name: "listed as resistant", chart: map[string]float64{"water": 0.5}, defender: "water", want: 0.5},
		{name: "listed as immune", chart: map[string]float64{"ghost": 0}, defender: "ghost", want: 0},
		{name: "absent from the chart", chart: map[string]float64{"grass": 2}, defender: "rock", want: 1},
		{name: "empty chart", chart: map[string]float64{}, defender: "rock", want: 1},
		{name: "nil chart", chart: nil, defender: "rock", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ty := Type{ID: "fire", Name: "Fire", Effectiveness: tt.chart}

			if got := ty.Against(tt.defender); got != tt.want {
				t.Errorf("Against(%q) = %v, want %v", tt.defender, got, tt.want)
			}
		})
	}
}

// TestTypeJSONFieldNames pins the wire format: these names are the API contract, and
// a typo in a json tag must fail here rather than in a client, weeks later.
func TestTypeJSONFieldNames(t *testing.T) {
	page := TypePage{Types: []Type{{ID: "fire", Name: "Fire", Effectiveness: map[string]float64{"grass": 2}}}}

	body, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}

	const want = `{"types":[{"id":"fire","name":"Fire","effectiveness":{"grass":2}}]}`
	if got := string(body); got != want {
		t.Errorf("Marshal() = %s\nwant           = %s", got, want)
	}

	page.NextCursor = "eyJwayI6IlRZUEUifQ"

	body, err = json.Marshal(page)
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}

	const wantWithCursor = `{"types":[{"id":"fire","name":"Fire","effectiveness":{"grass":2}}],` +
		`"nextCursor":"eyJwayI6IlRZUEUifQ"}`
	if got := string(body); got != wantWithCursor {
		t.Errorf("Marshal() = %s\nwant           = %s", got, wantWithCursor)
	}
}

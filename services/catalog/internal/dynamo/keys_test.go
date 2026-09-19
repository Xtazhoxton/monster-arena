package dynamo

import "testing"

// TestKeyShapes pins the literal key format down: it is the schema of ADR-0010,
// and data already written with another shape becomes unreachable.
func TestKeyShapes(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "creature partition", got: creaturePK("pikachu"), want: "CREATURE#pikachu"},
		{name: "move partition", got: movePK("thunderbolt"), want: "MOVE#thunderbolt"},
		{name: "type partition", got: typePK("electric"), want: "TYPE#electric"},
		{name: "type of a creature", got: creatureTypeSK("electric"), want: "TYPE#electric"},
		{name: "movepool entry", got: creatureMoveSK("surf"), want: "MOVE#surf"},
		{name: "effectiveness cell", got: versusSK("water"), want: "VS#water"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestIDAfter(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		entity string
		wantID string
		wantOK bool
	}{
		{name: "movepool entry", key: creatureMoveSK("surf"), entity: moveEntity, wantID: "surf", wantOK: true},
		{name: "type of a creature", key: creatureTypeSK("electric"), entity: typeEntity, wantID: "electric", wantOK: true},
		{name: "profile sort key", key: creatureEntity, entity: creatureEntity},
		{name: "another entity", key: creatureMoveSK("surf"), entity: typeEntity},
		{name: "empty identifier", key: "MOVE" + sep, entity: moveEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := idAfter(tt.key, tt.entity)
			if id != tt.wantID || ok != tt.wantOK {
				t.Errorf("idAfter(%q, %q) = (%q, %t), want (%q, %t)",
					tt.key, tt.entity, id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

package catalog

import (
	"errors"
	"testing"
)

// validCreature returns a creature every test case starts from.
func validCreature() Creature {
	return Creature{
		ID:         "pikachu",
		Name:       "Pikachu",
		Generation: 1,
		Types:      []string{"electric"},
		BaseStats: Stats{
			HP: 35, Attack: 55, Defense: 40,
			SpecialAttack: 50, SpecialDefense: 50, Speed: 90,
		},
		Moves: []LearnedMove{{MoveID: "thunderbolt", LearnMethod: "level-up", Level: 26}},
	}
}

func TestCreatureValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Creature)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Creature) {}},
		{name: "two types", mutate: func(c *Creature) { c.Types = []string{"fire", "flying"} }},
		{name: "empty id", mutate: func(c *Creature) { c.ID = "" }, wantErr: true},
		{name: "key injection in id", mutate: func(c *Creature) { c.ID = "pikachu#MOVE#surf" }, wantErr: true},
		{name: "uppercase id", mutate: func(c *Creature) { c.ID = "Pikachu" }, wantErr: true},
		{name: "empty name", mutate: func(c *Creature) { c.Name = "" }, wantErr: true},
		{name: "no type", mutate: func(c *Creature) { c.Types = nil }, wantErr: true},
		{name: "three types", mutate: func(c *Creature) { c.Types = []string{"fire", "flying", "water"} }, wantErr: true},
		{name: "duplicate types", mutate: func(c *Creature) { c.Types = []string{"fire", "fire"} }, wantErr: true},
		{name: "invalid move id", mutate: func(c *Creature) { c.Moves[0].MoveID = "Thunder Bolt" }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validCreature()
			tt.mutate(&c)

			err := c.Validate()
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

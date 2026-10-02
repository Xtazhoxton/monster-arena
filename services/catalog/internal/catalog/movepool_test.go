package catalog

import (
	"errors"
	"strings"
	"testing"
)

// validMovepool returns a movepool every test case starts from.
func validMovepool() Movepool {
	return Movepool{Moves: []LearnedMove{
		{MoveID: "thunderbolt", LearnMethod: LevelUp, Level: 26},
		{MoveID: "thunder-wave", LearnMethod: "machine"},
	}}
}

func TestMovepoolValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Movepool)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Movepool) {}},
		{name: "empty movepool", mutate: func(p *Movepool) { p.Moves = []LearnedMove{} }},
		{name: "nil movepool", mutate: func(p *Movepool) { p.Moves = nil }},
		// PokéAPI uses level 0 for the moves learned on evolution.
		{name: "level-up at level 0", mutate: func(p *Movepool) { p.Moves[0].Level = 0 }},
		{name: "unknown but well-formed method", mutate: func(p *Movepool) { p.Moves[1].LearnMethod = "stadium-surfing-pikachu" }},

		{name: "empty move id", mutate: func(p *Movepool) { p.Moves[0].MoveID = "" }, wantErr: true},
		{name: "key injection in move id", mutate: func(p *Movepool) { p.Moves[0].MoveID = "surf#TYPE#water" }, wantErr: true},
		{name: "same move twice", mutate: func(p *Movepool) { p.Moves[1].MoveID = "thunderbolt" }, wantErr: true},
		{name: "same move twice, different methods", mutate: func(p *Movepool) {
			p.Moves = append(p.Moves, LearnedMove{MoveID: "thunderbolt", LearnMethod: "machine"})
		}, wantErr: true},
		{name: "empty learn method", mutate: func(p *Movepool) { p.Moves[1].LearnMethod = "" }, wantErr: true},
		{name: "spaced learn method", mutate: func(p *Movepool) { p.Moves[1].LearnMethod = "level up" }, wantErr: true},
		{name: "negative level", mutate: func(p *Movepool) { p.Moves[0].Level = -1 }, wantErr: true},
		{name: "level on a machine move", mutate: func(p *Movepool) { p.Moves[1].Level = 12 }, wantErr: true},
		{name: "level 1 on a machine move", mutate: func(p *Movepool) { p.Moves[1].Level = 1 }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validMovepool()
			tt.mutate(&p)

			err := p.Validate()
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

// TestMovepoolValidateReportsEveryProblem checks that Validate accumulates its errors
// across entries instead of stopping at the first faulty one.
func TestMovepoolValidateReportsEveryProblem(t *testing.T) {
	p := Movepool{Moves: []LearnedMove{
		{MoveID: "Surf", LearnMethod: "machine"},
		{MoveID: "tackle", LearnMethod: "machine", Level: 5},
		{MoveID: "tackle", LearnMethod: ""},
	}}

	err := p.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error")
	}

	// Surf is not a slug; level on a machine move; tackle twice; empty method.
	const want = 4
	if got := len(strings.Split(err.Error(), "\n")); got != want {
		t.Errorf("Validate() reported %d problems, want %d:\n%v", got, want, err)
	}
}

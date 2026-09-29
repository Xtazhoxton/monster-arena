package catalog

import (
	"errors"
	"strings"
	"testing"
)

// validMove returns a move every test case starts from.
func validMove() Move {
	return Move{
		ID:          "thunderbolt",
		Name:        "Thunderbolt",
		Type:        "electric",
		DamageClass: Special,
		Power:       90,
		Accuracy:    100,
		PP:          15,
	}
}

func TestMoveValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Move)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Move) {}},
		{name: "physical", mutate: func(m *Move) { m.DamageClass = Physical }},
		{name: "status without power", mutate: func(m *Move) {
			m.DamageClass = Status
			m.Power = 0
		}},
		{name: "cannot miss", mutate: func(m *Move) { m.Accuracy = 0 }},
		{name: "damaging move without power", mutate: func(m *Move) { m.Power = 0 }},

		{name: "empty id", mutate: func(m *Move) { m.ID = "" }, wantErr: true},
		{name: "key injection in id", mutate: func(m *Move) { m.ID = "thunderbolt#TYPE#water" }, wantErr: true},
		{name: "uppercase id", mutate: func(m *Move) { m.ID = "Thunderbolt" }, wantErr: true},
		{name: "empty name", mutate: func(m *Move) { m.Name = "" }, wantErr: true},
		{name: "empty type", mutate: func(m *Move) { m.Type = "" }, wantErr: true},
		{name: "spaced type", mutate: func(m *Move) { m.Type = "sound wave" }, wantErr: true},
		{name: "empty damage class", mutate: func(m *Move) { m.DamageClass = "" }, wantErr: true},
		{name: "unknown damage class", mutate: func(m *Move) { m.DamageClass = "physcial" }, wantErr: true},
		{name: "uppercase damage class", mutate: func(m *Move) { m.DamageClass = "Special" }, wantErr: true},
		{name: "negative power", mutate: func(m *Move) { m.Power = -1 }, wantErr: true},
		{name: "status with power", mutate: func(m *Move) { m.DamageClass = Status }, wantErr: true},
		{name: "negative accuracy", mutate: func(m *Move) { m.Accuracy = -1 }, wantErr: true},
		{name: "accuracy above 100", mutate: func(m *Move) { m.Accuracy = 101 }, wantErr: true},
		{name: "zero pp", mutate: func(m *Move) { m.PP = 0 }, wantErr: true},
		{name: "negative pp", mutate: func(m *Move) { m.PP = -1 }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validMove()
			tt.mutate(&m)

			err := m.Validate()
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

// TestMoveValidateReportsEveryProblem checks that Validate accumulates its
// errors instead of stopping at the first one.
func TestMoveValidateReportsEveryProblem(t *testing.T) {
	m := Move{DamageClass: Status, Power: 40, Accuracy: 200, PP: 0}

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error")
	}

	// id, name, type, status-with-power, accuracy, pp.
	const want = 6
	if got := len(strings.Split(err.Error(), "\n")); got != want {
		t.Errorf("Validate() reported %d problems, want %d:\n%v", got, want, err)
	}
}

package dynamo

import (
	"testing"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

// TestMovepoolItemRoundTrip pins the keys of a movepool entry and checks that the
// domain value survives the trip through DynamoDB attribute names.
func TestMovepoolItemRoundTrip(t *testing.T) {
	want := catalog.LearnedMove{MoveID: "thunderbolt", LearnMethod: "level-up", Level: 26}

	item := newCreatureMoveItem("pikachu", want)
	if item.PK != "CREATURE#pikachu" || item.SK != "MOVE#thunderbolt" {
		t.Fatalf("keys = (%q, %q), want (%q, %q)",
			item.PK, item.SK, "CREATURE#pikachu", "MOVE#thunderbolt")
	}

	got, err := item.learnedMove()
	if err != nil {
		t.Fatalf("learnedMove() = %v, want nil", err)
	}

	if got != want {
		t.Errorf("round trip gave %+v, want %+v", got, want)
	}
}

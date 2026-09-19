package dynamo

import (
	"slices"
	"testing"
)

func TestRemoved(t *testing.T) {
	tests := []struct {
		name     string
		previous []string
		current  []string
		want     []string
	}{
		{name: "new creature", current: []string{"electric"}},
		{name: "unchanged", previous: []string{"electric"}, current: []string{"electric"}},
		{name: "second type dropped", previous: []string{"electric", "flying"}, current: []string{"electric"}, want: []string{"flying"}},
		{name: "second type added", previous: []string{"electric"}, current: []string{"electric", "flying"}},
		{name: "both replaced", previous: []string{"fire"}, current: []string{"water"}, want: []string{"fire"}},
		{name: "slots swapped", previous: []string{"electric", "flying"}, current: []string{"flying", "electric"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := removed(tt.previous, tt.current); !slices.Equal(got, tt.want) {
				t.Errorf("removed(%v, %v) = %v, want %v", tt.previous, tt.current, got, tt.want)
			}
		})
	}
}

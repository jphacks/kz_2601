package dsl

import (
	"strings"
	"testing"
)

func TestNearbyShareSemanticErrors(t *testing.T) {
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompNearbyShare, Label: "送る", Entity: "memo"}, // ok
		{Type: CompNearbyShare, Entity: "memo"},              // no label
		{Type: CompNearbyShare, Label: "送る", Entity: "nope"}, // unknown entity
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{"nearbyShare needs label", `nearbyShare entity "nope" does not exist`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
	if len(ve.Problems) != 2 {
		t.Errorf("want 2 problems, got %v", ve.Problems)
	}
}

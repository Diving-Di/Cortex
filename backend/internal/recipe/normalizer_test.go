package recipe

import (
	"reflect"
	"testing"
)

func TestNormalizeDietaryTerms(t *testing.T) {
	got := NormalizeDietaryTerms([]string{" 鱼、姜，蒜 ", "鱼", "100g"})
	want := []string{"姜", "蒜", "鱼"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

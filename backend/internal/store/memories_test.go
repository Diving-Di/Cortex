package store

import "testing"

func TestValidMemoryCategory(t *testing.T) {
	for _, value := range []string{"fact","preference","goal","habit","milestone"} {
		if !ValidMemoryCategory(value) { t.Fatalf("expected valid category %q",value) }
	}
	for _, value := range []string{"","profile","FACT"} {
		if ValidMemoryCategory(value) { t.Fatalf("expected invalid category %q",value) }
	}
}

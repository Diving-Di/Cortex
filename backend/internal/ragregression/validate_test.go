package ragregression

import "testing"

func TestValidatePublicFixture(t *testing.T) {
	if err := Validate("../../testdata/rag/regression/public_v1.jsonl", "../../testdata/rag/regression/public_v1_manifest.json"); err != nil {
		t.Fatal(err)
	}
}

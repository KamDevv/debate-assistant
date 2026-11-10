package ledger

import "testing"

func TestStableIDIsDeterministicAndNamespaced(t *testing.T) {
	first := stableID("clm", "debate|message|claim")
	second := stableID("clm", "debate|message|claim")
	if first != second || len(first) < 8 || first[:4] != "clm_" {
		t.Fatalf("unexpected stable IDs: %q and %q", first, second)
	}
	if first == stableID("evd", "debate|message|claim") {
		t.Fatal("different ID namespaces should not collide")
	}
}

func TestValidationHelpers(t *testing.T) {
	if validClaimType("made_up") != "factual" {
		t.Fatal("invalid claim type should fall back to factual")
	}
	if validAssessment("made_up") != "unverified" {
		t.Fatal("invalid assessment should fall back to unverified")
	}
	if validConfidence("made_up") != "low" {
		t.Fatal("invalid confidence should fall back to low")
	}
}

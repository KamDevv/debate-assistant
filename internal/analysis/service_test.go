package analysis

import "testing"

func TestFilterFindingSourcesOnlyKeepsKnownIDs(t *testing.T) {
	findings := []Finding{{SourceIDs: []string{"S1", "S2", "S1", "invented"}}}
	filterFindingSources(findings, map[string]bool{"S1": true})
	if len(findings[0].SourceIDs) != 1 || findings[0].SourceIDs[0] != "S1" {
		t.Fatalf("unexpected source IDs after validation: %#v", findings[0].SourceIDs)
	}
}

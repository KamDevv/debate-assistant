package research

import "testing"

func TestFilterIDsOnlyKeepsKnownUniqueIDs(t *testing.T) {
	got := filterIDs([]string{"S2", "S1", "S2", "fake"}, map[string]bool{"S1": true, "S2": true})
	if len(got) != 2 || got[0] != "S2" || got[1] != "S1" {
		t.Fatalf("unexpected IDs: %#v", got)
	}
}

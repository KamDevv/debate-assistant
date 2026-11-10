package store

import (
	"testing"

	"github.com/yourname/british-debate-assistant/internal/ledger"
)

func TestSaveAndGetLedger(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CreateDebate("debate-1", "Test debate", "Test topic"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddMessage("message-1", "debate-1", "you", "A test claim"); err != nil {
		t.Fatal(err)
	}

	report := ledger.Report{
		DebateID: "debate-1", MessageCount: 1, UpdatedAt: "2026-11-09T12:00:00Z",
		Warnings: []string{"test warning"},
		Claims: []ledger.Claim{
			{ID: "claim-a", DebateID: "debate-1", Speaker: "you", Text: "A test claim", ClaimType: "factual", Assessment: "supported", Confidence: "medium", Explanation: "A source supports it.", MessageIDs: []string{"message-1"}, FirstSeenAt: "2026-11-09T12:00:00Z", LastSeenAt: "2026-11-09T12:00:00Z", Evidence: []ledger.ClaimEvidence{
				{ID: "evidence-a", ClaimID: "claim-a", Title: "Example source", URL: "https://example.com/source", Domain: "example.com", Snippet: "Relevant extract", Relationship: "supports", Explanation: "The extract supports the claim.", Confidence: "medium", CreatedAt: "2026-11-09T12:00:00Z"},
			}},
			{ID: "claim-b", DebateID: "debate-1", Speaker: "opponent", Text: "The opposite claim", ClaimType: "factual", Assessment: "unverified", Confidence: "low", Explanation: "No source found.", MessageIDs: []string{"message-2"}, FirstSeenAt: "2026-11-09T12:00:00Z", LastSeenAt: "2026-11-09T12:00:00Z", Evidence: []ledger.ClaimEvidence{}},
		},
		Contradictions: []ledger.Contradiction{{ID: "contradiction-a", DebateID: "debate-1", ClaimAID: "claim-a", ClaimBID: "claim-b", Explanation: "They conflict.", Confidence: "medium", CreatedAt: "2026-11-09T12:00:00Z"}},
	}
	if err := db.SaveLedger(report); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetLedger("debate-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.MessageCount != 1 || len(got.Claims) != 2 || len(got.Contradictions) != 1 {
		t.Fatalf("unexpected ledger counts: messages=%d claims=%d contradictions=%d", got.MessageCount, len(got.Claims), len(got.Contradictions))
	}
	if got.Claims[0].ID != "claim-a" || len(got.Claims[0].Evidence) != 1 || got.Claims[0].MessageIDs[0] != "message-1" {
		t.Fatalf("claim or evidence was not restored correctly: %#v", got.Claims[0])
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != "test warning" {
		t.Fatalf("warnings were not restored: %#v", got.Warnings)
	}
}

func TestListMessagesAndTopic(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CreateDebate("debate-2", "Title", "Topic"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddMessage("message-2", "debate-2", "opponent", "Statement"); err != nil {
		t.Fatal(err)
	}
	topic, err := db.GetDebateTopic("debate-2")
	if err != nil || topic != "Topic" {
		t.Fatalf("GetDebateTopic = %q, %v", topic, err)
	}
	messages, err := db.ListMessages("debate-2")
	if err != nil || len(messages) != 1 || messages[0].Speaker != "opponent" {
		t.Fatalf("ListMessages = %#v, %v", messages, err)
	}
}

func TestUpdateMessageSpeaker(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CreateDebate("debate-a", "Debate A", "Topic"); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateDebate("debate-b", "Debate B", "Other topic"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddMessage("message-a", "debate-a", "opponent", "A statement"); err != nil {
		t.Fatal(err)
	}

	if err := db.SaveLedger(ledger.Report{
		DebateID: "debate-a", MessageCount: 1, UpdatedAt: "2026-11-10T12:00:00Z",
		Claims:         []ledger.Claim{{ID: "claim-a", DebateID: "debate-a", Speaker: "opponent", Text: "A statement", ClaimType: "factual", Assessment: "unverified", Confidence: "low", Explanation: "Not checked", MessageIDs: []string{"message-a"}, FirstSeenAt: "2026-11-10T12:00:00Z", LastSeenAt: "2026-11-10T12:00:00Z"}},
		Contradictions: []ledger.Contradiction{}, Warnings: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := db.UpdateMessageSpeaker("debate-a", "message-a", "you")
	if err != nil || !updated {
		t.Fatalf("UpdateMessageSpeaker = %v, %v; want true, nil", updated, err)
	}
	ledgerAfterCorrection, err := db.GetLedger("debate-a")
	if err != nil || len(ledgerAfterCorrection.Claims) != 0 || ledgerAfterCorrection.UpdatedAt != "" {
		t.Fatalf("speaker correction should invalidate the derived ledger: %#v, %v", ledgerAfterCorrection, err)
	}
	updated, err = db.UpdateMessageSpeaker("debate-b", "message-a", "opponent")
	if err != nil || updated {
		t.Fatalf("cross-debate update = %v, %v; want false, nil", updated, err)
	}
	messages, err := db.ListMessages("debate-a")
	if err != nil || len(messages) != 1 || messages[0].Speaker != "you" {
		t.Fatalf("speaker was not updated: %#v, %v", messages, err)
	}
}

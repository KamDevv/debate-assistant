package ledger

// Message is one saved transcript entry.
type Message struct {
	ID        string `json:"id"`
	Speaker   string `json:"speaker"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type Claim struct {
	ID          string          `json:"id"`
	DebateID    string          `json:"debate_id"`
	Speaker     string          `json:"speaker"`
	Text        string          `json:"text"`
	ClaimType   string          `json:"claim_type"`
	Assessment  string          `json:"assessment"`
	Confidence  string          `json:"confidence"`
	Explanation string          `json:"explanation"`
	MessageIDs  []string        `json:"message_ids"`
	FirstSeenAt string          `json:"first_seen_at"`
	LastSeenAt  string          `json:"last_seen_at"`
	Evidence    []ClaimEvidence `json:"evidence"`
}

type ClaimEvidence struct {
	ID           string `json:"id"`
	ClaimID      string `json:"claim_id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Domain       string `json:"domain,omitempty"`
	Snippet      string `json:"snippet"`
	PublishedAt  string `json:"published_at,omitempty"`
	Relationship string `json:"relationship"`
	Explanation  string `json:"explanation"`
	Confidence   string `json:"confidence"`
	CreatedAt    string `json:"created_at"`
}

type Contradiction struct {
	ID          string `json:"id"`
	DebateID    string `json:"debate_id"`
	ClaimAID    string `json:"claim_a_id"`
	ClaimBID    string `json:"claim_b_id"`
	Explanation string `json:"explanation"`
	Confidence  string `json:"confidence"`
	CreatedAt   string `json:"created_at"`
}

type Report struct {
	DebateID       string          `json:"debate_id"`
	Claims         []Claim         `json:"claims"`
	Contradictions []Contradiction `json:"contradictions"`
	MessageCount   int             `json:"message_count"`
	UpdatedAt      string          `json:"updated_at,omitempty"`
	Warnings       []string        `json:"warnings"`
}

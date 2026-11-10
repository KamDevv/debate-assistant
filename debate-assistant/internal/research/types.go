package research

import "github.com/yourname/british-debate-assistant/internal/evidence"

type Request struct {
	Topic    string `json:"topic"`
	Position string `json:"position,omitempty"`
}

type Argument struct {
	Title     string   `json:"title"`
	Reasoning string   `json:"reasoning"`
	Evidence  string   `json:"evidence,omitempty"`
	SourceIDs []string `json:"source_ids,omitempty"`
}

type Fact struct {
	Claim      string   `json:"claim"`
	Assessment string   `json:"assessment"` // supported, contested, context_needed, not_established, or opinion
	Context    string   `json:"context"`
	Confidence string   `json:"confidence"`
	SourceIDs  []string `json:"source_ids,omitempty"`
}

type Report struct {
	ID               string            `json:"id,omitempty"`
	Topic            string            `json:"topic"`
	Position         string            `json:"position,omitempty"`
	GeneratedAt      string            `json:"generated_at"`
	Overview         string            `json:"overview"`
	ArgumentsFor     []Argument        `json:"arguments_for"`
	ArgumentsAgainst []Argument        `json:"arguments_against"`
	KeyFacts         []Fact            `json:"key_facts"`
	Counterarguments []string          `json:"counterarguments_to_expect"`
	RisksToPosition  []string          `json:"risks_to_your_position"`
	Questions        []string          `json:"questions_to_ask"`
	Caveats          []string          `json:"caveats"`
	DevilsAdvocate   string            `json:"devils_advocate"`
	OpeningStatement string            `json:"opening_statement"`
	ClosingStatement string            `json:"closing_statement"`
	RebuttalBank     []Rebuttal        `json:"rebuttal_bank"`
	EvidenceGaps     []string          `json:"evidence_gaps"`
	DebateStrategy   []string          `json:"debate_strategy"`
	Sources          []evidence.Source `json:"sources"`
	Disclaimer       string            `json:"disclaimer"`
}

type Rebuttal struct {
	Challenge string   `json:"challenge"`
	Response  string   `json:"response"`
	Caveat    string   `json:"caveat,omitempty"`
	SourceIDs []string `json:"source_ids,omitempty"`
}

type Summary struct {
	ID        string `json:"id"`
	Topic     string `json:"topic"`
	Position  string `json:"position,omitempty"`
	CreatedAt string `json:"created_at"`
}

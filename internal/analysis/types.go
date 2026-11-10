package analysis

import "github.com/yourname/british-debate-assistant/internal/evidence"

type Message struct {
	Speaker string `json:"speaker"`
	Content string `json:"content"`
}

type Request struct {
	Topic           string    `json:"topic"`
	Messages        []Message `json:"messages"`
	LatestSpeaker   string    `json:"latest_speaker"`
	LatestStatement string    `json:"latest_statement"`
}

type Finding struct {
	Category          string   `json:"category"`
	Severity          string   `json:"severity"`
	Title             string   `json:"title"`
	Explanation       string   `json:"explanation"`
	SuggestedResponse string   `json:"suggested_response,omitempty"`
	Question          string   `json:"question,omitempty"`
	Confidence        string   `json:"confidence"`
	SourceIDs         []string `json:"source_ids,omitempty"`
}

type Result struct {
	Summary          string            `json:"summary"`
	OpponentFindings []Finding         `json:"opponent_findings"`
	YourFindings     []Finding         `json:"your_findings"`
	Suggestions      []string          `json:"suggestions"`
	EvidenceNeeded   []string          `json:"evidence_needed"`
	Sources          []evidence.Source `json:"sources"`
	ResearchUsed     bool              `json:"research_used"`
	ResearchWarning  string            `json:"research_warning,omitempty"`
	Disclaimer       string            `json:"disclaimer"`
}

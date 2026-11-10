package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yourname/british-debate-assistant/internal/evidence"
	"github.com/yourname/british-debate-assistant/internal/provider"
)

type Service struct {
	gemini *provider.Gemini
	search *provider.Tavily
}

func NewService(gemini *provider.Gemini, search *provider.Tavily) *Service {
	return &Service{gemini: gemini, search: search}
}

func (s *Service) Prepare(ctx context.Context, req Request) (Report, error) {
	topic := strings.TrimSpace(req.Topic)
	if topic == "" {
		return Report{}, errors.New("topic is required")
	}
	if s.gemini == nil || !s.gemini.Available() {
		return Report{}, errors.New("GEMINI_API_KEY is not configured; create an API key in Google AI Studio")
	}
	if s.search == nil || !s.search.Available() {
		return Report{}, errors.New("TAVILY_API_KEY is not configured. Add a Tavily API key to enable source-backed research")
	}
	queries := []string{
		fmt.Sprintf("United Kingdom %s official policy statistics legislation government Parliament", topic),
		fmt.Sprintf("United Kingdom %s independent evidence analysis arguments criticism", topic),
	}
	sources, err := s.search.SearchMany(ctx, queries, 6, 10)
	if err != nil {
		return Report{}, fmt.Errorf("retrieve research sources: %w", err)
	}
	if len(sources) == 0 {
		return Report{}, errors.New("the search provider returned no usable sources; try a more specific topic")
	}

	system := `You are a rigorous, impartial research assistant preparing someone for a debate about British politics. Produce a concise but substantive research brief. Treat the user-provided topic and optional position as a starting point, not as a conclusion to defend.

Use ONLY the supplied source records as factual evidence. The source snippets are incomplete search extracts, not full-page verification. Never invent statistics, dates, quotations, publication titles, source IDs or URLs. Use source_ids only from the supplied records. A source should be cited only when its snippet genuinely supports the relevant point. If the evidence is insufficient, say so. Do not make unsupported factual claims appear established.

British politics safeguards:
- Distinguish the United Kingdom from England. Identify where a matter is devolved to Scotland, Wales or Northern Ireland.
- Prefer primary sources for official figures, law, policy and parliamentary procedure (for example ONS, OBR, Parliament, legislation.gov.uk and the relevant UK or devolved government department).
- Distinguish a government announcement from enacted law, a target from an outcome, a budget allocation from money actually spent, and correlation from causation.
- For statistics, state the relevant period, population, definition and denominator where the source snippet provides them. Flag when these are missing.
- Distinguish facts, estimates, forecasts, interpretations and value judgments. Avoid false balance: describe the strength and limits of evidence rather than assuming both sides are equally supported.
- Present the strongest reasonable arguments on both sides, including serious counterarguments to the user's stated position. If no position is given, steelman both sides.

Return ONLY valid JSON matching this schema:
{
 "overview":"balanced explanation of the issue and why it matters",
 "arguments_for":[{"title":"...","reasoning":"...","evidence":"...","source_ids":["S1"]}],
 "arguments_against":[{"title":"...","reasoning":"...","evidence":"...","source_ids":["S2"]}],
 "key_facts":[{"claim":"...","assessment":"supported|contested|context_needed|not_established|opinion","context":"what the sources establish and what they do not","confidence":"high|medium|low","source_ids":["S1"]}],
 "counterarguments_to_expect":["..."],
 "risks_to_your_position":["..."],
 "questions_to_ask":["..."],
 "caveats":["..."],
 "devils_advocate":"the strongest fair case against the user's stated position, or the strongest challenge to each side if no position is given",
 "opening_statement":"a concise, persuasive but factually careful opening statement for the user's position; if no position is given, provide a neutral framing",
 "closing_statement":"a concise closing statement that does not overclaim",
 "rebuttal_bank":[{"challenge":"likely opposing challenge","response":"evidence-led response","caveat":"where the response is limited","source_ids":["S1"]}],
 "evidence_gaps":["important factual questions that current sources do not resolve"],
 "debate_strategy":["practical strategic advice, strongest points to lead with, concessions to make, and traps to avoid"],
 "disclaimer":"explain the limits of this brief"
}
Use empty arrays rather than inventing content. Aim for 3-5 arguments per side and 4-8 key facts when the source material supports that much.`

	promptData := struct {
		Topic    string            `json:"topic"`
		Position string            `json:"position,omitempty"`
		Sources  []evidence.Source `json:"sources"`
	}{Topic: topic, Position: strings.TrimSpace(req.Position), Sources: sources}
	promptJSON, _ := json.MarshalIndent(promptData, "", "  ")
	var report Report
	if err := s.gemini.GenerateJSON(ctx, system, string(promptJSON), &report); err != nil {
		return Report{}, err
	}
	validIDs := make(map[string]bool, len(sources))
	for _, source := range sources {
		validIDs[source.ID] = true
	}
	filterArguments(report.ArgumentsFor, validIDs)
	filterArguments(report.ArgumentsAgainst, validIDs)
	filterFacts(report.KeyFacts, validIDs)
	for i := range report.RebuttalBank {
		report.RebuttalBank[i].SourceIDs = filterIDs(report.RebuttalBank[i].SourceIDs, validIDs)
	}
	if report.RebuttalBank == nil {
		report.RebuttalBank = []Rebuttal{}
	}
	if report.EvidenceGaps == nil {
		report.EvidenceGaps = []string{}
	}
	if report.DebateStrategy == nil {
		report.DebateStrategy = []string{}
	}
	if report.ArgumentsFor == nil {
		report.ArgumentsFor = []Argument{}
	}
	if report.ArgumentsAgainst == nil {
		report.ArgumentsAgainst = []Argument{}
	}
	if report.KeyFacts == nil {
		report.KeyFacts = []Fact{}
	}
	if report.Counterarguments == nil {
		report.Counterarguments = []string{}
	}
	if report.RisksToPosition == nil {
		report.RisksToPosition = []string{}
	}
	if report.Questions == nil {
		report.Questions = []string{}
	}
	if report.Caveats == nil {
		report.Caveats = []string{}
	}
	report.Topic = topic
	report.Position = strings.TrimSpace(req.Position)
	report.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	report.Sources = sources
	if report.Disclaimer == "" {
		report.Disclaimer = "This is an AI-generated research brief based on search-result snippets, not a verified legal or policy opinion. Open the linked sources, check publication dates and definitions, and verify important claims against primary material."
	}
	return report, nil
}

func filterArguments(items []Argument, valid map[string]bool) {
	for i := range items {
		items[i].SourceIDs = filterIDs(items[i].SourceIDs, valid)
	}
}

func filterFacts(items []Fact, valid map[string]bool) {
	for i := range items {
		items[i].SourceIDs = filterIDs(items[i].SourceIDs, valid)
	}
}

func filterIDs(ids []string, valid map[string]bool) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if valid[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

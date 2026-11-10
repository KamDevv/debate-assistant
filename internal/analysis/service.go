package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

func (s *Service) Analyse(ctx context.Context, req Request) (Result, error) {
	if s.gemini == nil || !s.gemini.Available() {
		return Result{}, fmt.Errorf("GEMINI_API_KEY is not configured; create an API key in Google AI Studio")
	}
	var sources []evidence.Source
	var researchWarning string
	if s.search != nil && s.search.Available() {
		query := buildClaimSearchQuery(req)
		found, err := s.search.Search(ctx, query, 6)
		if err != nil {
			researchWarning = "Live web research could not be completed for this analysis. The AI assessment is not source-verified. " + err.Error()
		} else {
			sources = found
		}
	} else {
		researchWarning = "Live source checking is not configured. Add TAVILY_API_KEY to enable evidence retrieval."
	}

	system := `You are an impartial, strategically useful British political debate analyst. Assess both participants to the same evidential standard. Help the user respond effectively, but never validate a claim merely because it supports the user's position.

Separate verifiable factual claims, opinions, predictions, interpretations and value judgments. Do not call a claim false merely because the supplied sources do not mention it. A missing source means unverified, not disproved. Identify material omissions, definitions, dates, denominators, comparisons, causal assumptions and uncertainty. Distinguish UK-wide policy from England-only policy and identify devolved responsibilities where relevant. Do not invent statistics, quotations, sources or citations.
Prioritise the latest statement for immediate debate use. If the opponent spoke last, make the first suggestion a concise, speakable response to their strongest point, followed where useful by a precise question. If the user spoke last, identify the most important correction or qualification they should make. Keep suggestions practical and brief; distinguish a strong rebuttal from a point that still needs evidence.

The user message includes a list of source records with IDs. Cite only those exact IDs in source_ids. Do not create source IDs. If a source does not actually support the point, do not cite it. Treat search snippets as leads, not as a substitute for reading the full source. When sources conflict, explain the disagreement rather than selecting whichever side is convenient.

Return ONLY valid JSON matching this schema:
{
 "summary":"short summary",
 "opponent_findings":[{"category":"factual|context|logic|unsupported|contradiction|evasion","severity":"high|medium|low","title":"...","explanation":"...","suggested_response":"...","question":"...","confidence":"high|medium|low","source_ids":["S1"]}],
 "your_findings":[{"category":"factual|context|logic|unsupported|contradiction|evasion","severity":"high|medium|low","title":"...","explanation":"...","suggested_response":"...","question":"...","confidence":"high|medium|low","source_ids":["S1"]}],
 "suggestions":["concise strategic response"],
 "evidence_needed":["specific evidence or data needed to verify a claim"],
 "disclaimer":"explain what is and is not verified"
}
Use empty arrays when appropriate. A fallacy label must include a clear explanation and should be used cautiously. Do not pretend every disagreement is a factual error. If no sources are supplied, explicitly say that factual claims remain unverified.`

	prompt := formatRequest(req, sources)
	var result Result
	if err := s.gemini.GenerateJSON(ctx, system, prompt, &result); err != nil {
		return Result{}, err
	}
	validIDs := make(map[string]bool, len(sources))
	for _, source := range sources {
		validIDs[source.ID] = true
	}
	filterFindingSources(result.OpponentFindings, validIDs)
	filterFindingSources(result.YourFindings, validIDs)
	if result.OpponentFindings == nil {
		result.OpponentFindings = []Finding{}
	}
	if result.YourFindings == nil {
		result.YourFindings = []Finding{}
	}
	if result.Suggestions == nil {
		result.Suggestions = []string{}
	}
	if result.EvidenceNeeded == nil {
		result.EvidenceNeeded = []string{}
	}
	result.Sources = sources
	result.ResearchUsed = len(sources) > 0
	result.ResearchWarning = researchWarning
	if result.Disclaimer == "" {
		result.Disclaimer = "AI analysis is provisional. Search snippets are not conclusive proof; open primary sources and check dates, definitions and context before relying on a claim."
	}
	return result, nil
}

func buildClaimSearchQuery(req Request) string {
	statement := strings.Join(strings.Fields(req.LatestStatement), " ")
	if len(statement) > 320 {
		statement = statement[:320]
	}
	query := fmt.Sprintf("United Kingdom %s verify evidence statistics policy: %s", req.Topic, statement)
	if len(query) > 480 {
		query = query[:480]
	}
	return query
}

func formatRequest(req Request, sources []evidence.Source) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Debate topic: %s\nLatest speaker: %s\nLatest statement: %s\n\nConversation so far:\n", req.Topic, req.LatestSpeaker, req.LatestStatement)
	for _, message := range req.Messages {
		fmt.Fprintf(&b, "[%s]: %s\n", message.Speaker, message.Content)
	}
	if len(sources) == 0 {
		b.WriteString("\nNo live web sources were retrieved. Do not say that any claim has been checked against external evidence.\n")
	} else {
		b.WriteString("\nRetrieved web sources (use only these source IDs; snippets may be incomplete):\n")
		encoded, _ := json.MarshalIndent(sources, "", "  ")
		b.Write(encoded)
	}
	return b.String()
}

func filterFindingSources(findings []Finding, valid map[string]bool) {
	for i := range findings {
		filtered := make([]string, 0, len(findings[i].SourceIDs))
		seen := map[string]bool{}
		for _, id := range findings[i].SourceIDs {
			if valid[id] && !seen[id] {
				seen[id] = true
				filtered = append(filtered, id)
			}
		}
		findings[i].SourceIDs = filtered
	}
}

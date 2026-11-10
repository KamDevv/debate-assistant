package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
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

type extraction struct {
	Claims []struct {
		Speaker        string `json:"speaker"`
		Text           string `json:"text"`
		ClaimType      string `json:"claim_type"`
		MessageNumbers []int  `json:"message_numbers"`
	} `json:"claims"`
	Contradictions []struct {
		ClaimANumber int    `json:"claim_a_number"`
		ClaimBNumber int    `json:"claim_b_number"`
		Explanation  string `json:"explanation"`
		Confidence   string `json:"confidence"`
	} `json:"contradictions"`
}

type assessmentOutput struct {
	Assessments []struct {
		ClaimNumber int    `json:"claim_number"`
		Assessment  string `json:"assessment"`
		Confidence  string `json:"confidence"`
		Explanation string `json:"explanation"`
		Evidence    []struct {
			SourceID     string `json:"source_id"`
			Relationship string `json:"relationship"`
			Explanation  string `json:"explanation"`
		} `json:"evidence"`
	} `json:"assessments"`
}

// Analyse extracts claims from the complete transcript, compares them for
// potential contradictions, searches a small number of factual claims, and
// attaches the retrieved evidence to the persistent ledger.
func (s *Service) Analyse(ctx context.Context, debateID, topic string, messages []Message) (Report, error) {
	if s.gemini == nil || !s.gemini.Available() {
		return Report{}, fmt.Errorf("GEMINI_API_KEY is not configured; create an API key in Google AI Studio")
	}
	if len(messages) == 0 {
		return Report{}, fmt.Errorf("add at least one transcript statement before updating the argument ledger")
	}
	if len(messages) > 100 {
		return Report{}, fmt.Errorf("the argument ledger supports up to 100 transcript statements per update")
	}

	transcript := make([]map[string]any, 0, len(messages))
	for i, message := range messages {
		transcript = append(transcript, map[string]any{
			"message_number": i + 1,
			"speaker":        message.Speaker,
			"content":        message.Content,
		})
	}
	input, _ := json.MarshalIndent(map[string]any{"topic": topic, "transcript": transcript}, "", "  ")

	extractionSystem := `You maintain an impartial argument ledger for a British political debate. Read the COMPLETE transcript and extract its important, distinct, checkable or debate-relevant propositions.

Rules:
- Extract atomic claims, not whole paragraphs. Keep the wording faithful to what the speaker actually asserted; do not strengthen or weaken it.
- Attribute each claim to the speaker who made it. Use only speaker values "you" or "opponent".
- Use message_numbers (1-based) to identify every transcript message that contains that same claim by that same speaker. Combine repeated statements where they make the same proposition; do not combine different speakers into one claim.
- Include factual claims, predictions, interpretations, value judgements and policy proposals when they are important to the debate. Avoid extracting rhetorical filler and questions unless they contain a substantive assertion.
- claim_type must be factual, prediction, interpretation, value_judgement, or proposal.
- Identify contradictions between extracted claims only when the propositions are genuinely incompatible in the same relevant context, time period and definitions. A changed position, qualification, different population, or different time period is not automatically a contradiction. Contradictions can be within one speaker or between speakers.
- Contradiction claim_a_number and claim_b_number are 1-based positions in the claims array. Do not refer to nonexistent claims.
- Be selective: usually 5-25 important claims, not every sentence. Preserve nuance and uncertainty.

Return ONLY valid JSON:
{"claims":[{"speaker":"you|opponent","text":"...","claim_type":"factual|prediction|interpretation|value_judgement|proposal","message_numbers":[1,2]}],"contradictions":[{"claim_a_number":1,"claim_b_number":2,"explanation":"...","confidence":"high|medium|low"}]}
Use empty arrays when no relevant claims or contradictions exist.`

	var extracted extraction
	if err := s.gemini.GenerateJSON(ctx, extractionSystem, string(input), &extracted); err != nil {
		return Report{}, fmt.Errorf("extract debate claims: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	claims := make([]Claim, 0, len(extracted.Claims))
	claimIDs := make(map[int]string, len(extracted.Claims))
	claimIndexByID := make(map[string]int, len(extracted.Claims))
	messageByNumber := make(map[int]Message, len(messages))
	for i, message := range messages {
		messageByNumber[i+1] = message
	}
	for i, item := range extracted.Claims {
		text := strings.TrimSpace(item.Text)
		if text == "" || len(text) > 2000 {
			continue
		}
		messageIDs := make([]string, 0, len(item.MessageNumbers))
		firstSeen, lastSeen := "", ""
		messageNumbers := append([]int(nil), item.MessageNumbers...)
		sort.Ints(messageNumbers)
		seen := map[string]bool{}
		actualSpeaker := ""
		for _, number := range messageNumbers {
			message, ok := messageByNumber[number]
			if !ok || (message.Speaker != "you" && message.Speaker != "opponent") {
				continue
			}
			if actualSpeaker == "" {
				actualSpeaker = message.Speaker
			}
			if message.Speaker != actualSpeaker || message.Speaker != item.Speaker || message.ID == "" || seen[message.ID] {
				continue
			}
			seen[message.ID] = true
			messageIDs = append(messageIDs, message.ID)
			seenAt := message.CreatedAt
			if seenAt == "" {
				seenAt = now
			}
			if firstSeen == "" || seenAt < firstSeen {
				firstSeen = seenAt
			}
			if lastSeen == "" || seenAt > lastSeen {
				lastSeen = seenAt
			}
		}
		if len(messageIDs) == 0 || (item.Speaker != "you" && item.Speaker != "opponent") {
			continue
		}
		if firstSeen == "" {
			firstSeen = now
		}
		if lastSeen == "" {
			lastSeen = now
		}
		claimType := validClaimType(item.ClaimType)
		id := stableID("clm", debateID+"\x00"+item.Speaker+"\x00"+normalise(text))
		claimIDs[i+1] = id
		if existingIndex, exists := claimIndexByID[id]; exists {
			for _, messageID := range messageIDs {
				alreadyAdded := false
				for _, existingID := range claims[existingIndex].MessageIDs {
					if existingID == messageID {
						alreadyAdded = true
						break
					}
				}
				if !alreadyAdded {
					claims[existingIndex].MessageIDs = append(claims[existingIndex].MessageIDs, messageID)
				}
			}
			if firstSeen < claims[existingIndex].FirstSeenAt {
				claims[existingIndex].FirstSeenAt = firstSeen
			}
			if lastSeen > claims[existingIndex].LastSeenAt {
				claims[existingIndex].LastSeenAt = lastSeen
			}
			continue
		}
		claimIndexByID[id] = len(claims)
		assessment := "unverified"
		if claimType == "value_judgement" || claimType == "proposal" {
			assessment = "opinion"
		}
		claims = append(claims, Claim{
			ID: id, DebateID: debateID, Speaker: item.Speaker, Text: text,
			ClaimType: claimType, Assessment: assessment, Confidence: "low",
			Explanation: "Awaiting source assessment.", MessageIDs: messageIDs,
			FirstSeenAt: firstSeen, LastSeenAt: lastSeen, Evidence: []ClaimEvidence{},
		})
	}

	contradictions := make([]Contradiction, 0, len(extracted.Contradictions))
	seenPairs := map[string]bool{}
	for _, item := range extracted.Contradictions {
		a, okA := claimIDs[item.ClaimANumber]
		b, okB := claimIDs[item.ClaimBNumber]
		if !okA || !okB || a == b || strings.TrimSpace(item.Explanation) == "" {
			continue
		}
		if b < a {
			a, b = b, a
		}
		pair := a + "\x00" + b
		if seenPairs[pair] {
			continue
		}
		seenPairs[pair] = true
		contradictions = append(contradictions, Contradiction{
			ID: stableID("con", debateID+"\x00"+pair), DebateID: debateID,
			ClaimAID: a, ClaimBID: b, Explanation: strings.TrimSpace(item.Explanation),
			Confidence: validConfidence(item.Confidence), CreatedAt: now,
		})
	}

	warnings := []string{}
	sources := []evidence.Source{}
	queries := make([]string, 0, 6)
	for _, claim := range claims {
		if claim.ClaimType != "factual" && claim.ClaimType != "prediction" {
			continue
		}
		query := strings.TrimSpace("United Kingdom " + topic + " " + claim.Text + " official evidence statistics")
		if len(query) > 420 {
			query = query[:420]
		}
		queries = append(queries, query)
		if len(queries) == 6 {
			break
		}
	}
	if len(queries) > 0 {
		if s.search == nil || !s.search.Available() {
			warnings = append(warnings, "Web evidence is not configured. Factual claims remain unverified; add TAVILY_API_KEY to retrieve sources.")
		} else {
			found, err := s.search.SearchMany(ctx, queries, 3, 12)
			if err != nil {
				warnings = append(warnings, "Live source retrieval failed. Claims have been saved without source verification: "+err.Error())
			} else {
				sources = found
				if len(sources) == 0 {
					warnings = append(warnings, "No usable web sources were returned for the selected factual claims. They remain unverified.")
				}
			}
		}
	}

	if len(sources) > 0 {
		for i := range claims {
			if claims[i].ClaimType == "value_judgement" || claims[i].ClaimType == "proposal" {
				claims[i].Assessment = "opinion"
				claims[i].Confidence = "low"
				claims[i].Explanation = "This is a value judgement rather than a directly verifiable factual claim."
				continue
			}
			claims[i].Assessment = "unverified"
			claims[i].Confidence = "low"
			claims[i].Explanation = "The retrieved search snippets have not yet established this claim."
		}
		if err := s.assessClaims(ctx, claims, sources); err != nil {
			warnings = append(warnings, "Claims were extracted, but source assessment could not be completed: "+err.Error())
			for i := range claims {
				if claims[i].ClaimType == "value_judgement" || claims[i].ClaimType == "proposal" {
					continue
				}
				claims[i].Assessment = "unverified"
				claims[i].Confidence = "low"
				claims[i].Explanation = "Source assessment did not complete. This claim has not been verified or disproved."
			}
		}
	} else {
		for i := range claims {
			if claims[i].ClaimType == "value_judgement" || claims[i].ClaimType == "proposal" {
				claims[i].Assessment = "opinion"
				claims[i].Explanation = "This is a value judgement rather than a directly verifiable factual claim."
			} else {
				claims[i].Assessment = "unverified"
				claims[i].Explanation = "No live sources were available to assess this claim. This does not mean the claim is false."
			}
		}
	}

	return Report{
		DebateID: debateID, Claims: claims, Contradictions: contradictions,
		MessageCount: len(messages), UpdatedAt: now, Warnings: warnings,
	}, nil
}

func (s *Service) assessClaims(ctx context.Context, claims []Claim, sources []evidence.Source) error {
	claimInputs := make([]map[string]any, 0, len(claims))
	for i, claim := range claims {
		claimInputs = append(claimInputs, map[string]any{
			"claim_number": i + 1, "speaker": claim.Speaker, "text": claim.Text,
			"claim_type": claim.ClaimType,
		})
	}
	input, _ := json.MarshalIndent(map[string]any{"claims": claimInputs, "sources": sources}, "", "  ")
	system := `Assess each debate claim using ONLY the supplied web-search source records. Search snippets are incomplete leads, not full-page verification.

For each claim:
- assessment must be supported, contested, contradicted, context_needed, unverified, or opinion.
- supported means at least one supplied source directly supports the substance of the claim.
- contested means relevant supplied sources materially disagree or the evidence is mixed.
- contradicted means a reliable supplied source clearly conflicts with the claim in the same context and period.
- context_needed means the claim may be broadly true but omits a material date, denominator, definition, jurisdiction, comparison or qualification.
- unverified means the supplied records do not establish the claim. This does NOT mean it is false.
- opinion is for value judgements; do not pretend that evidence proves a moral or political preference.
- Do not infer that a source supports a claim from its title alone. Never invent source IDs, facts, dates, quotations or source contents.
- Link only sources that genuinely relate to the claim. relationship must be supports, challenges, or context. Explain briefly what the snippet does and does not establish.
- Apply the same standard to both speakers. Be especially careful about UK versus England-only policy and devolved responsibilities.

Return ONLY valid JSON:
{"assessments":[{"claim_number":1,"assessment":"supported|contested|contradicted|context_needed|unverified|opinion","confidence":"high|medium|low","explanation":"...","evidence":[{"source_id":"S1","relationship":"supports|challenges|context","explanation":"..."}]}]}
Include one assessment for each claim. Use empty evidence arrays when no source genuinely supports or challenges it.`
	var assessed assessmentOutput
	if err := s.gemini.GenerateJSON(ctx, system, string(input), &assessed); err != nil {
		return err
	}
	validSources := make(map[string]evidence.Source, len(sources))
	for _, source := range sources {
		validSources[source.ID] = source
	}
	for _, item := range assessed.Assessments {
		if item.ClaimNumber < 1 || item.ClaimNumber > len(claims) {
			continue
		}
		claim := &claims[item.ClaimNumber-1]
		if claim.ClaimType == "value_judgement" || claim.ClaimType == "proposal" {
			claim.Assessment = "opinion"
			continue
		}
		claim.Assessment = validAssessment(item.Assessment)
		claim.Confidence = validConfidence(item.Confidence)
		claim.Explanation = strings.TrimSpace(item.Explanation)
		if claim.Explanation == "" {
			claim.Explanation = "The available source snippets do not provide a conclusive assessment."
		}
		if claim.Assessment == "opinion" {
			continue
		}
		for _, link := range item.Evidence {
			source, ok := validSources[link.SourceID]
			if !ok || strings.TrimSpace(link.Explanation) == "" {
				continue
			}
			relationship := link.Relationship
			if relationship != "supports" && relationship != "challenges" && relationship != "context" {
				continue
			}
			claim.Evidence = append(claim.Evidence, ClaimEvidence{
				ID: stableID("evd", claim.ID+"\x00"+source.URL), ClaimID: claim.ID,
				Title: source.Title, URL: source.URL, Domain: source.Domain,
				Snippet: source.Snippet, PublishedAt: source.PublishedAt,
				Relationship: relationship, Explanation: strings.TrimSpace(link.Explanation),
				Confidence: claim.Confidence, CreatedAt: time.Now().UTC().Format(time.RFC3339),
			})
		}
	}
	return nil
}

func validClaimType(value string) string {
	switch value {
	case "factual", "prediction", "interpretation", "value_judgement", "proposal":
		return value
	default:
		return "factual"
	}
}

func validAssessment(value string) string {
	switch value {
	case "supported", "contested", "contradicted", "context_needed", "unverified", "opinion":
		return value
	default:
		return "unverified"
	}
}

func validConfidence(value string) string {
	switch value {
	case "high", "medium", "low":
		return value
	default:
		return "low"
	}
}

func normalise(value string) string { return strings.ToLower(strings.Join(strings.Fields(value), " ")) }

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(sum[:10])
}

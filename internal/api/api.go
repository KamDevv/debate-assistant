package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/yourname/british-debate-assistant/internal/analysis"
	"github.com/yourname/british-debate-assistant/internal/ledger"
	"github.com/yourname/british-debate-assistant/internal/provider"
	"github.com/yourname/british-debate-assistant/internal/research"
	"github.com/yourname/british-debate-assistant/internal/store"
)

type API struct {
	store      *store.Store
	analyser   *analysis.Service
	researcher *research.Service
	ledgerer   *ledger.Service
	gemini     *provider.Gemini
	search     *provider.Tavily
}

func New(s *store.Store) http.Handler {
	gemini := provider.NewGemini()
	search := provider.NewTavily()
	a := &API{
		store: s, gemini: gemini, search: search,
		analyser:   analysis.NewService(gemini, search),
		researcher: research.NewService(gemini, search),
		ledgerer:   ledger.NewService(gemini, search),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("GET /api/debates", a.listDebates)
	mux.HandleFunc("POST /api/debates", a.createDebate)
	mux.HandleFunc("GET /api/debates/{id}/messages", a.listMessages)
	mux.HandleFunc("POST /api/debates/{id}/messages", a.addMessage)
	mux.HandleFunc("PATCH /api/debates/{id}/messages/{messageID}", a.updateMessageSpeaker)
	mux.HandleFunc("GET /api/debates/{id}/ledger", a.getLedger)
	mux.HandleFunc("POST /api/debates/{id}/ledger/analyse", a.analyseLedger)
	mux.HandleFunc("POST /api/analyse", a.analyse)
	mux.HandleFunc("GET /api/research", a.listResearch)
	mux.HandleFunc("POST /api/research", a.createResearch)
	mux.HandleFunc("GET /api/research/{id}", a.getResearch)
	return cors(mux)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, http.StatusOK, map[string]any{
		"status": "ok", "ai_provider": "Google Gemini",
		"gemini_configured":       a.gemini.Available(),
		"gemini_model":            a.gemini.Model(),
		"web_research_configured": a.search.Available(),
	})
}

func (a *API) createDebate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title string `json:"title"`
		Topic string `json:"topic"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Title, in.Topic = strings.TrimSpace(in.Title), strings.TrimSpace(in.Topic)
	if in.Title == "" || in.Topic == "" {
		fail(w, http.StatusBadRequest, "title and topic are required")
		return
	}
	if len(in.Title) > 160 || len(in.Topic) > 2000 {
		fail(w, http.StatusBadRequest, "title or topic is too long")
		return
	}
	id, err := newID()
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not create debate ID")
		return
	}
	if err := a.store.CreateDebate(id, in.Title, in.Topic); err != nil {
		fail(w, http.StatusInternalServerError, "could not create debate")
		return
	}
	jsonOut(w, http.StatusCreated, map[string]string{"id": id, "title": in.Title, "topic": in.Topic})
}

func (a *API) listDebates(w http.ResponseWriter, r *http.Request) {
	rows, err := a.store.ListDebates()
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not list debates")
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var id, title, topic, created string
		if rows.Scan(&id, &title, &topic, &created) == nil {
			out = append(out, map[string]string{"id": id, "title": title, "topic": topic, "created_at": created})
		}
	}
	jsonOut(w, http.StatusOK, out)
}

func (a *API) addMessage(w http.ResponseWriter, r *http.Request) {
	debateID := r.PathValue("id")
	exists, err := a.store.DebateExists(debateID)
	if err != nil || !exists {
		fail(w, http.StatusNotFound, "debate not found")
		return
	}
	var in struct {
		Speaker string `json:"speaker"`
		Content string `json:"content"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Content = strings.TrimSpace(in.Content)
	if (in.Speaker != "you" && in.Speaker != "opponent") || in.Content == "" {
		fail(w, http.StatusBadRequest, "speaker must be you or opponent and content is required")
		return
	}
	if len(in.Content) > 12000 {
		fail(w, http.StatusBadRequest, "message is too long (maximum 12,000 characters)")
		return
	}
	id, err := newID()
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not create message ID")
		return
	}
	if err := a.store.AddMessage(id, debateID, in.Speaker, in.Content); err != nil {
		fail(w, http.StatusInternalServerError, "could not save message")
		return
	}
	jsonOut(w, http.StatusCreated, map[string]string{"id": id, "speaker": in.Speaker, "content": in.Content})
}

func (a *API) updateMessageSpeaker(w http.ResponseWriter, r *http.Request) {
	debateID := r.PathValue("id")
	messageID := r.PathValue("messageID")
	exists, err := a.store.DebateExists(debateID)
	if err != nil || !exists {
		fail(w, http.StatusNotFound, "debate not found")
		return
	}
	var in struct {
		Speaker string `json:"speaker"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Speaker != "you" && in.Speaker != "opponent" {
		fail(w, http.StatusBadRequest, "speaker must be you or opponent")
		return
	}
	updated, err := a.store.UpdateMessageSpeaker(debateID, messageID, in.Speaker)
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not update transcript speaker")
		return
	}
	if !updated {
		fail(w, http.StatusNotFound, "transcript message not found")
		return
	}
	jsonOut(w, http.StatusOK, map[string]string{"id": messageID, "speaker": in.Speaker})
}

func (a *API) listMessages(w http.ResponseWriter, r *http.Request) {
	debateID := r.PathValue("id")
	exists, err := a.store.DebateExists(debateID)
	if err != nil || !exists {
		fail(w, http.StatusNotFound, "debate not found")
		return
	}
	rows, err := a.store.GetMessages(debateID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not retrieve messages")
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var id, speaker, content, created string
		if rows.Scan(&id, &speaker, &content, &created) == nil {
			out = append(out, map[string]string{"id": id, "speaker": speaker, "content": content, "created_at": created})
		}
	}
	jsonOut(w, http.StatusOK, out)
}

func (a *API) analyse(w http.ResponseWriter, r *http.Request) {
	var in analysis.Request
	if !decode(w, r, &in) {
		return
	}
	in.Topic = strings.TrimSpace(in.Topic)
	in.LatestStatement = strings.TrimSpace(in.LatestStatement)
	if in.Topic == "" || in.LatestStatement == "" {
		fail(w, http.StatusBadRequest, "topic and latest_statement are required")
		return
	}
	if len(in.Topic) > 2000 || len(in.LatestStatement) > 12000 || len(in.Messages) > 100 {
		fail(w, http.StatusBadRequest, "analysis request exceeds allowed limits")
		return
	}
	result, err := a.analyser.Analyse(r.Context(), in)
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "GEMINI_API_KEY is not configured") {
			status = http.StatusServiceUnavailable
		}
		fail(w, status, err.Error())
		return
	}
	jsonOut(w, http.StatusOK, result)
}

func (a *API) getLedger(w http.ResponseWriter, r *http.Request) {
	debateID := r.PathValue("id")
	exists, err := a.store.DebateExists(debateID)
	if err != nil || !exists {
		fail(w, http.StatusNotFound, "debate not found")
		return
	}
	report, err := a.store.GetLedger(debateID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not retrieve argument ledger")
		return
	}
	jsonOut(w, http.StatusOK, report)
}

func (a *API) analyseLedger(w http.ResponseWriter, r *http.Request) {
	debateID := r.PathValue("id")
	exists, err := a.store.DebateExists(debateID)
	if err != nil || !exists {
		fail(w, http.StatusNotFound, "debate not found")
		return
	}
	messages, err := a.store.ListMessages(debateID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not load transcript for argument ledger")
		return
	}
	if len(messages) == 0 {
		fail(w, http.StatusBadRequest, "add at least one transcript statement before updating the argument ledger")
		return
	}
	if len(messages) > 100 {
		fail(w, http.StatusBadRequest, "the argument ledger supports up to 100 transcript statements per update")
		return
	}
	totalBytes := 0
	for _, message := range messages {
		totalBytes += len(message.Content)
	}
	if totalBytes > 120000 {
		fail(w, http.StatusRequestEntityTooLarge, "the transcript is too large for one ledger update (maximum 120,000 bytes)")
		return
	}
	topic, err := a.store.GetDebateTopic(debateID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not load debate topic")
		return
	}
	transcript := make([]ledger.Message, 0, len(messages))
	for _, message := range messages {
		transcript = append(transcript, ledger.Message{ID: message.ID, Speaker: message.Speaker, Content: message.Content, CreatedAt: message.CreatedAt})
	}
	report, err := a.ledgerer.Analyse(r.Context(), debateID, topic, transcript)
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "GEMINI_API_KEY is not configured") {
			status = http.StatusServiceUnavailable
		}
		fail(w, status, err.Error())
		return
	}
	if err := a.store.SaveLedger(report); err != nil {
		fail(w, http.StatusInternalServerError, "could not save argument ledger")
		return
	}
	saved, err := a.store.GetLedger(debateID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "ledger was analysed but could not be reloaded")
		return
	}
	jsonOut(w, http.StatusOK, saved)
}

func (a *API) createResearch(w http.ResponseWriter, r *http.Request) {
	var in research.Request
	if !decode(w, r, &in) {
		return
	}
	in.Topic = strings.TrimSpace(in.Topic)
	in.Position = strings.TrimSpace(in.Position)
	if in.Topic == "" {
		fail(w, http.StatusBadRequest, "topic is required")
		return
	}
	if len(in.Topic) > 2000 || len(in.Position) > 2000 {
		fail(w, http.StatusBadRequest, "topic or position is too long")
		return
	}
	report, err := a.researcher.Prepare(r.Context(), in)
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "TAVILY_API_KEY is not configured") || strings.Contains(err.Error(), "GEMINI_API_KEY is not configured") {
			status = http.StatusServiceUnavailable
		}
		fail(w, status, err.Error())
		return
	}
	report.ID, err = newID()
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not create research ID")
		return
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not encode research report")
		return
	}
	if err := a.store.CreateResearch(report.ID, report.Topic, report.Position, string(encoded)); err != nil {
		fail(w, http.StatusInternalServerError, "could not save research report")
		return
	}
	jsonOut(w, http.StatusCreated, report)
}

func (a *API) listResearch(w http.ResponseWriter, r *http.Request) {
	rows, err := a.store.ListResearch()
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not list research reports")
		return
	}
	defer rows.Close()
	out := []research.Summary{}
	for rows.Next() {
		var item research.Summary
		if rows.Scan(&item.ID, &item.Topic, &item.Position, &item.CreatedAt) == nil {
			out = append(out, item)
		}
	}
	jsonOut(w, http.StatusOK, out)
}

func (a *API) getResearch(w http.ResponseWriter, r *http.Request) {
	record, err := a.store.GetResearch(r.PathValue("id"))
	if err != nil {
		if err == sql.ErrNoRows {
			fail(w, http.StatusNotFound, "research report not found")
			return
		}
		fail(w, http.StatusInternalServerError, "could not retrieve research report")
		return
	}
	var report research.Report
	if err := json.Unmarshal([]byte(record.Report), &report); err != nil {
		fail(w, http.StatusInternalServerError, "saved research report is invalid")
		return
	}
	jsonOut(w, http.StatusOK, report)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON request or request body too large")
		return false
	}
	return true
}

func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	jsonOut(w, status, map[string]string{"error": msg})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(raw[0:4]), hex.EncodeToString(raw[4:6]),
		hex.EncodeToString(raw[6:8]), hex.EncodeToString(raw[8:10]),
		hex.EncodeToString(raw[10:16])), nil
}

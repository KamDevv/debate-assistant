package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/yourname/british-debate-assistant/internal/ledger"
	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }

type Message struct {
	ID        string `json:"id"`
	Speaker   string `json:"speaker"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type ResearchRecord struct {
	ID        string
	Topic     string
	Position  string
	Report    string
	CreatedAt string
}

func (s *Store) Close() error { return s.DB.Close() }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}
	schema := `
	CREATE TABLE IF NOT EXISTS debates (
		id TEXT PRIMARY KEY, title TEXT NOT NULL, topic TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY, debate_id TEXT NOT NULL, speaker TEXT NOT NULL,
		content TEXT NOT NULL, created_at TEXT NOT NULL,
		FOREIGN KEY(debate_id) REFERENCES debates(id)
	);
	CREATE TABLE IF NOT EXISTS research_runs (
		id TEXT PRIMARY KEY, topic TEXT NOT NULL, position TEXT NOT NULL,
		report_json TEXT NOT NULL, created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS debate_claims (
		id TEXT PRIMARY KEY, debate_id TEXT NOT NULL, speaker TEXT NOT NULL,
		text TEXT NOT NULL, claim_type TEXT NOT NULL, assessment TEXT NOT NULL,
		confidence TEXT NOT NULL, explanation TEXT NOT NULL, message_ids_json TEXT NOT NULL,
		first_seen_at TEXT NOT NULL, last_seen_at TEXT NOT NULL,
		FOREIGN KEY(debate_id) REFERENCES debates(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_debate_claims_debate ON debate_claims(debate_id, first_seen_at);
	CREATE TABLE IF NOT EXISTS claim_evidence (
		id TEXT PRIMARY KEY, claim_id TEXT NOT NULL, title TEXT NOT NULL,
		url TEXT NOT NULL, domain TEXT NOT NULL, snippet TEXT NOT NULL,
		published_at TEXT NOT NULL, relationship TEXT NOT NULL,
		explanation TEXT NOT NULL, confidence TEXT NOT NULL, created_at TEXT NOT NULL,
		UNIQUE(claim_id, url),
		FOREIGN KEY(claim_id) REFERENCES debate_claims(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS claim_contradictions (
		id TEXT PRIMARY KEY, debate_id TEXT NOT NULL, claim_a_id TEXT NOT NULL,
		claim_b_id TEXT NOT NULL, explanation TEXT NOT NULL, confidence TEXT NOT NULL,
		created_at TEXT NOT NULL, UNIQUE(debate_id, claim_a_id, claim_b_id),
		FOREIGN KEY(debate_id) REFERENCES debates(id) ON DELETE CASCADE,
		FOREIGN KEY(claim_a_id) REFERENCES debate_claims(id) ON DELETE CASCADE,
		FOREIGN KEY(claim_b_id) REFERENCES debate_claims(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS debate_ledger_state (
		debate_id TEXT PRIMARY KEY, message_count INTEGER NOT NULL,
		updated_at TEXT NOT NULL, warnings_json TEXT NOT NULL,
		FOREIGN KEY(debate_id) REFERENCES debates(id) ON DELETE CASCADE
	);`
	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

func (s *Store) CreateDebate(id, title, topic string) error {
	_, err := s.DB.Exec(`INSERT INTO debates(id,title,topic,created_at) VALUES(?,?,?,?)`,
		id, title, topic, time.Now().UTC().Format(time.RFC3339))
	return err
}
func (s *Store) ListDebates() (*sql.Rows, error) {
	return s.DB.Query(`SELECT id,title,topic,created_at FROM debates ORDER BY created_at DESC`)
}
func (s *Store) AddMessage(id, debateID, speaker, content string) error {
	_, err := s.DB.Exec(`INSERT INTO messages(id,debate_id,speaker,content,created_at) VALUES(?,?,?,?,?)`,
		id, debateID, speaker, content, time.Now().UTC().Format(time.RFC3339))
	return err
}
func (s *Store) GetMessages(debateID string) (*sql.Rows, error) {
	return s.DB.Query(`SELECT id,speaker,content,created_at FROM messages WHERE debate_id=? ORDER BY rowid`, debateID)
}

// UpdateMessageSpeaker changes a transcript entry's speaker label. Since claim
// attribution depends on speakers, the derived ledger is cleared in the same
// transaction and can be rebuilt from the corrected transcript.
func (s *Store) UpdateMessageSpeaker(debateID, messageID, speaker string) (bool, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`UPDATE messages SET speaker=? WHERE id=? AND debate_id=?`, speaker, messageID, debateID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		return false, nil
	}
	if _, err := tx.Exec(`DELETE FROM debate_claims WHERE debate_id=?`, debateID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM debate_ledger_state WHERE debate_id=?`, debateID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
func (s *Store) DebateExists(id string) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM debates WHERE id=?`, id).Scan(&n)
	return n > 0, err
}

func (s *Store) CreateResearch(id, topic, position, reportJSON string) error {
	_, err := s.DB.Exec(`INSERT INTO research_runs(id,topic,position,report_json,created_at) VALUES(?,?,?,?,?)`,
		id, topic, position, reportJSON, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) ListResearch() (*sql.Rows, error) {
	return s.DB.Query(`SELECT id,topic,position,created_at FROM research_runs ORDER BY created_at DESC LIMIT 25`)
}

func (s *Store) GetResearch(id string) (ResearchRecord, error) {
	var record ResearchRecord
	err := s.DB.QueryRow(`SELECT id,topic,position,report_json,created_at FROM research_runs WHERE id=?`, id).
		Scan(&record.ID, &record.Topic, &record.Position, &record.Report, &record.CreatedAt)
	return record, err
}

func (s *Store) GetDebateTopic(id string) (string, error) {
	var topic string
	err := s.DB.QueryRow(`SELECT topic FROM debates WHERE id=?`, id).Scan(&topic)
	return topic, err
}

func (s *Store) ListMessages(debateID string) ([]Message, error) {
	rows, err := s.DB.Query(`SELECT id,speaker,content,created_at FROM messages WHERE debate_id=? ORDER BY rowid`, debateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.ID, &message.Speaker, &message.Content, &message.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	return out, rows.Err()
}

// SaveLedger persists the current analysis as one transaction. Existing claims
// and evidence are updated in place so repeated refreshes do not create copies.
func (s *Store) SaveLedger(report ledger.Report) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, claim := range report.Claims {
		messageIDs, err := json.Marshal(claim.MessageIDs)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO debate_claims(
			id,debate_id,speaker,text,claim_type,assessment,confidence,explanation,
			message_ids_json,first_seen_at,last_seen_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			text=excluded.text, claim_type=excluded.claim_type, assessment=excluded.assessment,
			confidence=excluded.confidence, explanation=excluded.explanation,
			message_ids_json=excluded.message_ids_json,
			first_seen_at=MIN(debate_claims.first_seen_at, excluded.first_seen_at),
			last_seen_at=MAX(debate_claims.last_seen_at, excluded.last_seen_at)`,
			claim.ID, report.DebateID, claim.Speaker, claim.Text, claim.ClaimType,
			claim.Assessment, claim.Confidence, claim.Explanation, string(messageIDs),
			claim.FirstSeenAt, claim.LastSeenAt)
		if err != nil {
			return err
		}
		for _, item := range claim.Evidence {
			_, err = tx.Exec(`INSERT INTO claim_evidence(
				id,claim_id,title,url,domain,snippet,published_at,relationship,explanation,confidence,created_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(claim_id,url) DO UPDATE SET
				title=excluded.title, domain=excluded.domain, snippet=excluded.snippet,
				published_at=excluded.published_at, relationship=excluded.relationship,
				explanation=excluded.explanation, confidence=excluded.confidence,
				created_at=excluded.created_at`,
				item.ID, claim.ID, item.Title, item.URL, item.Domain, item.Snippet,
				item.PublishedAt, item.Relationship, item.Explanation, item.Confidence, item.CreatedAt)
			if err != nil {
				return err
			}
		}
	}

	if _, err := tx.Exec(`DELETE FROM claim_contradictions WHERE debate_id=?`, report.DebateID); err != nil {
		return err
	}
	for _, item := range report.Contradictions {
		_, err = tx.Exec(`INSERT INTO claim_contradictions(
			id,debate_id,claim_a_id,claim_b_id,explanation,confidence,created_at
		) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(debate_id,claim_a_id,claim_b_id) DO UPDATE SET
			explanation=excluded.explanation, confidence=excluded.confidence, created_at=excluded.created_at`,
			item.ID, report.DebateID, item.ClaimAID, item.ClaimBID,
			item.Explanation, item.Confidence, item.CreatedAt)
		if err != nil {
			return err
		}
	}

	warnings, err := json.Marshal(report.Warnings)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO debate_ledger_state(debate_id,message_count,updated_at,warnings_json)
		VALUES(?,?,?,?) ON CONFLICT(debate_id) DO UPDATE SET
		message_count=excluded.message_count, updated_at=excluded.updated_at, warnings_json=excluded.warnings_json`,
		report.DebateID, report.MessageCount, report.UpdatedAt, string(warnings))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetLedger(debateID string) (ledger.Report, error) {
	report := ledger.Report{
		DebateID: debateID, Claims: []ledger.Claim{},
		Contradictions: []ledger.Contradiction{}, Warnings: []string{},
	}
	var warningsJSON string
	err := s.DB.QueryRow(`SELECT message_count,updated_at,warnings_json FROM debate_ledger_state WHERE debate_id=?`, debateID).
		Scan(&report.MessageCount, &report.UpdatedAt, &warningsJSON)
	if err != nil && err != sql.ErrNoRows {
		return report, err
	}
	if warningsJSON != "" {
		_ = json.Unmarshal([]byte(warningsJSON), &report.Warnings)
	}

	rows, err := s.DB.Query(`SELECT id,debate_id,speaker,text,claim_type,assessment,confidence,explanation,
		message_ids_json,first_seen_at,last_seen_at FROM debate_claims WHERE debate_id=? ORDER BY first_seen_at,id`, debateID)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var claim ledger.Claim
		var messageIDsJSON string
		if err := rows.Scan(&claim.ID, &claim.DebateID, &claim.Speaker, &claim.Text,
			&claim.ClaimType, &claim.Assessment, &claim.Confidence, &claim.Explanation,
			&messageIDsJSON, &claim.FirstSeenAt, &claim.LastSeenAt); err != nil {
			rows.Close()
			return report, err
		}
		_ = json.Unmarshal([]byte(messageIDsJSON), &claim.MessageIDs)
		if claim.MessageIDs == nil {
			claim.MessageIDs = []string{}
		}
		claim.Evidence = []ledger.ClaimEvidence{}
		report.Claims = append(report.Claims, claim)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()

	for i := range report.Claims {
		evidenceRows, err := s.DB.Query(`SELECT id,claim_id,title,url,domain,snippet,published_at,
			relationship,explanation,confidence,created_at FROM claim_evidence WHERE claim_id=? ORDER BY created_at DESC`, report.Claims[i].ID)
		if err != nil {
			return report, err
		}
		for evidenceRows.Next() {
			var item ledger.ClaimEvidence
			if err := evidenceRows.Scan(&item.ID, &item.ClaimID, &item.Title, &item.URL,
				&item.Domain, &item.Snippet, &item.PublishedAt, &item.Relationship,
				&item.Explanation, &item.Confidence, &item.CreatedAt); err != nil {
				evidenceRows.Close()
				return report, err
			}
			report.Claims[i].Evidence = append(report.Claims[i].Evidence, item)
		}
		if err := evidenceRows.Err(); err != nil {
			evidenceRows.Close()
			return report, err
		}
		evidenceRows.Close()
	}

	contradictionRows, err := s.DB.Query(`SELECT id,debate_id,claim_a_id,claim_b_id,explanation,confidence,created_at
		FROM claim_contradictions WHERE debate_id=? ORDER BY created_at DESC`, debateID)
	if err != nil {
		return report, err
	}
	defer contradictionRows.Close()
	for contradictionRows.Next() {
		var item ledger.Contradiction
		if err := contradictionRows.Scan(&item.ID, &item.DebateID, &item.ClaimAID,
			&item.ClaimBID, &item.Explanation, &item.Confidence, &item.CreatedAt); err != nil {
			return report, err
		}
		report.Contradictions = append(report.Contradictions, item)
	}
	return report, contradictionRows.Err()
}

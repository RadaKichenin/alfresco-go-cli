package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/aborroy/alfresco-cli/internal/validation"
)

type ApprovalRecord struct {
	ApprovalID   string
	OperationID  string
	TraceID      string
	PlanID       string
	PlanHash     string
	Status       string
	Reason       string
	RequestedBy  string
	RequestedAt  time.Time
	DecidedBy    string
	DecidedAt    time.Time
	DecisionNote string
	ApplyRequest validation.ApplyRequest
}

type Store struct {
	db *sql.DB
}

type ApprovalListFilter struct {
	Status      string
	RequestedBy string
	Limit       int
	Offset      int
}

func NewSQLite(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) init() error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS approvals (
			approval_id TEXT PRIMARY KEY,
			operation_id TEXT NOT NULL,
			trace_id TEXT NOT NULL,
			plan_id TEXT NOT NULL,
			plan_hash TEXT NOT NULL,
			status TEXT NOT NULL,
			reason TEXT,
			requested_by TEXT NOT NULL,
			requested_at TEXT NOT NULL,
			decided_by TEXT,
			decided_at TEXT,
			decision_note TEXT,
			apply_request_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS idempotency (
			idempotency_key TEXT PRIMARY KEY,
			operation_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS operations (
			operation_id TEXT PRIMARY KEY,
			operation_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
	}
	for _, q := range schema {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("schema init failed: %w", err)
		}
	}
	return nil
}

func (s *Store) CreateApproval(r ApprovalRecord) error {
	r.Status = "pending"
	if r.RequestedAt.IsZero() {
		r.RequestedAt = time.Now().UTC()
	}
	applyJSON, err := json.Marshal(r.ApplyRequest)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO approvals (
		approval_id, operation_id, trace_id, plan_id, plan_hash, status, reason,
		requested_by, requested_at, decided_by, decided_at, decision_note, apply_request_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ApprovalID, r.OperationID, r.TraceID, r.PlanID, r.PlanHash, r.Status, r.Reason,
		r.RequestedBy, r.RequestedAt.Format(time.RFC3339Nano), nullIfEmpty(r.DecidedBy), nullableTime(r.DecidedAt), nullIfEmpty(r.DecisionNote), string(applyJSON),
	)
	return err
}

func (s *Store) GetApproval(id string) (ApprovalRecord, bool, error) {
	row := s.db.QueryRow(`SELECT approval_id, operation_id, trace_id, plan_id, plan_hash, status, reason,
		requested_by, requested_at, decided_by, decided_at, decision_note, apply_request_json
		FROM approvals WHERE approval_id = ?`, id)
	var r ApprovalRecord
	var requestedAt string
	var decidedAt sql.NullString
	var applyReqJSON string
	var decidedBy sql.NullString
	var decisionNote sql.NullString
	if err := row.Scan(&r.ApprovalID, &r.OperationID, &r.TraceID, &r.PlanID, &r.PlanHash, &r.Status, &r.Reason,
		&r.RequestedBy, &requestedAt, &decidedBy, &decidedAt, &decisionNote, &applyReqJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ApprovalRecord{}, false, nil
		}
		return ApprovalRecord{}, false, err
	}
	if t, err := time.Parse(time.RFC3339Nano, requestedAt); err == nil {
		r.RequestedAt = t
	}
	if decidedBy.Valid {
		r.DecidedBy = decidedBy.String
	}
	if decidedAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, decidedAt.String); err == nil {
			r.DecidedAt = t
		}
	}
	if decisionNote.Valid {
		r.DecisionNote = decisionNote.String
	}
	if applyReqJSON != "" {
		_ = json.Unmarshal([]byte(applyReqJSON), &r.ApplyRequest)
	}
	return r, true, nil
}

func (s *Store) ListApprovals(filter ApprovalListFilter) ([]ApprovalRecord, error) {
	where := []string{"1=1"}
	args := []any{}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.RequestedBy != "" {
		where = append(where, "requested_by = ?")
		args = append(args, filter.RequestedBy)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	q := fmt.Sprintf(`SELECT approval_id, operation_id, trace_id, plan_id, plan_hash, status, reason,
		requested_by, requested_at, decided_by, decided_at, decision_note, apply_request_json
		FROM approvals
		WHERE %s
		ORDER BY requested_at DESC
		LIMIT ? OFFSET ?`, strings.Join(where, " AND "))
	args = append(args, limit, offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ApprovalRecord, 0, limit)
	for rows.Next() {
		var r ApprovalRecord
		var requestedAt string
		var decidedAt sql.NullString
		var applyReqJSON string
		var decidedBy sql.NullString
		var decisionNote sql.NullString
		if err := rows.Scan(&r.ApprovalID, &r.OperationID, &r.TraceID, &r.PlanID, &r.PlanHash, &r.Status, &r.Reason,
			&r.RequestedBy, &requestedAt, &decidedBy, &decidedAt, &decisionNote, &applyReqJSON); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339Nano, requestedAt); err == nil {
			r.RequestedAt = t
		}
		if decidedBy.Valid {
			r.DecidedBy = decidedBy.String
		}
		if decidedAt.Valid {
			if t, err := time.Parse(time.RFC3339Nano, decidedAt.String); err == nil {
				r.DecidedAt = t
			}
		}
		if decisionNote.Valid {
			r.DecisionNote = decisionNote.String
		}
		if applyReqJSON != "" {
			_ = json.Unmarshal([]byte(applyReqJSON), &r.ApplyRequest)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DecideApproval(id, actor, decision, reason string, at time.Time) (ApprovalRecord, error) {
	r, ok, err := s.GetApproval(id)
	if err != nil {
		return ApprovalRecord{}, err
	}
	if !ok {
		return ApprovalRecord{}, errors.New("approval not found")
	}
	if r.Status != "pending" {
		return ApprovalRecord{}, errors.New("approval already decided")
	}
	status := ""
	switch decision {
	case "approve":
		status = "approved"
	case "reject":
		status = "rejected"
	default:
		return ApprovalRecord{}, errors.New("invalid decision")
	}
	_, err = s.db.Exec(`UPDATE approvals SET status=?, decided_by=?, decided_at=?, decision_note=? WHERE approval_id=?`,
		status, actor, at.Format(time.RFC3339Nano), nullIfEmpty(reason), id,
	)
	if err != nil {
		return ApprovalRecord{}, err
	}
	r.Status = status
	r.DecidedBy = actor
	r.DecidedAt = at
	r.DecisionNote = reason
	return r, nil
}

func (s *Store) PutOperation(status validation.OperationStatusResponse) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO operations(operation_id, operation_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(operation_id) DO UPDATE SET operation_json=excluded.operation_json, updated_at=excluded.updated_at`,
		status.OperationID, string(raw), time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetOperation(id string) (validation.OperationStatusResponse, bool, error) {
	row := s.db.QueryRow(`SELECT operation_json FROM operations WHERE operation_id=?`, id)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return validation.OperationStatusResponse{}, false, nil
		}
		return validation.OperationStatusResponse{}, false, err
	}
	var out validation.OperationStatusResponse
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return validation.OperationStatusResponse{}, false, err
	}
	return out, true, nil
}

func (s *Store) PutIdempotency(key string, status validation.OperationStatusResponse) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO idempotency(idempotency_key, operation_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(idempotency_key) DO UPDATE SET operation_json=excluded.operation_json, updated_at=excluded.updated_at`,
		key, string(raw), time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetIdempotency(key string) (validation.OperationStatusResponse, bool, error) {
	row := s.db.QueryRow(`SELECT operation_json FROM idempotency WHERE idempotency_key=?`, key)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return validation.OperationStatusResponse{}, false, nil
		}
		return validation.OperationStatusResponse{}, false, err
	}
	var out validation.OperationStatusResponse
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return validation.OperationStatusResponse{}, false, err
	}
	return out, true, nil
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339Nano)
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

package approval

import (
	"errors"
	"sync"
	"time"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

type Record struct {
	ApprovalID   string
	OperationID  string
	TraceID      string
	PlanID       string
	PlanHash     string
	Status       Status
	Reason       string
	RequestedBy  string
	RequestedAt  time.Time
	DecidedBy    string
	DecidedAt    time.Time
	DecisionNote string
}

type Store struct {
	mu      sync.RWMutex
	records map[string]Record
}

func NewStore() *Store {
	return &Store{records: make(map[string]Record)}
}

func (s *Store) Create(r Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.Status = StatusPending
	s.records[r.ApprovalID] = r
}

func (s *Store) Get(id string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[id]
	return r, ok
}

func (s *Store) Decide(id, actor, decision, reason string, at time.Time) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.records[id]
	if !ok {
		return Record{}, errors.New("approval not found")
	}
	if r.Status != StatusPending {
		return Record{}, errors.New("approval already decided")
	}
	switch decision {
	case "approve":
		r.Status = StatusApproved
	case "reject":
		r.Status = StatusRejected
	default:
		return Record{}, errors.New("invalid decision")
	}
	r.DecidedBy = actor
	r.DecidedAt = at
	r.DecisionNote = reason
	s.records[id] = r
	return r, nil
}

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aborroy/alfresco-cli/internal/approval"
	"github.com/aborroy/alfresco-cli/internal/audit"
	"github.com/aborroy/alfresco-cli/internal/executor"
	"github.com/aborroy/alfresco-cli/internal/validation"
)

type pendingApply struct {
	Req         validation.ApplyRequest
	TraceID     string
	OperationID string
}

type server struct {
	exec     executor.Executor
	approval *approval.Store
	audit    *audit.Logger

	mu             sync.RWMutex
	idempotency    map[string]validation.OperationStatusResponse
	operations     map[string]validation.OperationStatusResponse
	pendingApplies map[string]pendingApply
}

func main() {
	var addr string
	var auditLogPath string
	var agentBin string
	flag.StringVar(&addr, "addr", ":8090", "HTTP listen address")
	flag.StringVar(&auditLogPath, "audit-log", "./audit.log", "Audit log JSONL path")
	flag.StringVar(&agentBin, "agent-bin", "/root/.zeroclaw/workspace/skills/alfresco-agent/bin/alfresco-agent", "alfresco-agent binary path")
	flag.Parse()

	s := &server{
		exec:           executor.NewCLIExecutor(agentBin),
		approval:       approval.NewStore(),
		audit:          audit.NewLogger(auditLogPath),
		idempotency:    make(map[string]validation.OperationStatusResponse),
		operations:     make(map[string]validation.OperationStatusResponse),
		pendingApplies: make(map[string]pendingApply),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/resolve", s.handleResolve)
	mux.HandleFunc("/v1/plan", s.handlePlan)
	mux.HandleFunc("/v1/apply", s.handleApply)
	mux.HandleFunc("/v1/approvals/", s.handleApprovalDecision)
	mux.HandleFunc("/v1/operations/", s.handleGetOperation)
	mux.HandleFunc("/v1/audit/", s.handleGetAudit)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	h := withJSONContentType(withTraceMiddleware(mux))
	log.Printf("alfresco agent gateway listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}

func (s *server) handleResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported", traceIDFromContext(r.Context()), nil)
		return
	}
	traceID := traceIDFromContext(r.Context())

	var req validation.ResolveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), traceID, nil)
		return
	}
	if errs := validation.ValidateResolveRequest(req); len(errs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "request validation failed", traceID, errs)
		return
	}

	resp, err := s.exec.Resolve(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "resolve_failed", err.Error(), traceID, nil)
		return
	}
	if err := validation.ValidateResolveResponseDeterministic(resp); err != nil {
		writeError(w, http.StatusInternalServerError, "non_deterministic_resolve_output", err.Error(), traceID, nil)
		return
	}

	s.logEvent(traceID, "resolve_completed", actorFromRequest(r), map[string]any{"status": resp.Status, "request_id": resp.RequestID})
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported", traceIDFromContext(r.Context()), nil)
		return
	}
	traceID := traceIDFromContext(r.Context())
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required", traceID, nil)
		return
	}
	if prior, ok := s.getIdempotent("plan:" + key); ok {
		writeJSON(w, http.StatusOK, prior)
		return
	}

	var req validation.PlanRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), traceID, nil)
		return
	}
	if errs := validation.ValidatePlanRequest(req); len(errs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "request validation failed", traceID, errs)
		return
	}

	resp, err := s.exec.Plan(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "plan_failed", err.Error(), traceID, nil)
		return
	}

	op := validation.OperationStatusResponse{OperationID: resp.PlanID, Status: "planned", TraceID: traceID, Result: resp}
	s.storeOperation(op)
	s.storeIdempotent("plan:"+key, op)
	s.logEvent(traceID, "plan_created", actorFromRequest(r), map[string]any{"plan_id": resp.PlanID})
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported", traceIDFromContext(r.Context()), nil)
		return
	}
	traceID := traceIDFromContext(r.Context())
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required", traceID, nil)
		return
	}
	if prior, ok := s.getIdempotent("apply:" + key); ok {
		writeJSON(w, http.StatusAccepted, prior)
		return
	}

	var req validation.ApplyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), traceID, nil)
		return
	}
	if errs := validation.ValidateApplyRequest(req); len(errs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "request validation failed", traceID, errs)
		return
	}

	approvalID := newUUID()
	operationID := newUUID()
	s.approval.Create(approval.Record{
		ApprovalID:  approvalID,
		OperationID: operationID,
		TraceID:     traceID,
		PlanID:      req.PlanID,
		PlanHash:    req.PlanHash,
		RequestedBy: actorFromRequest(r),
		RequestedAt: time.Now().UTC(),
	})
	s.mu.Lock()
	s.pendingApplies[approvalID] = pendingApply{Req: req, TraceID: traceID, OperationID: operationID}
	s.mu.Unlock()

	resp := validation.ApplyResponse{
		SchemaVersion: validation.SchemaVersion,
		RequestID:     "apply-" + shortID(),
		ExecutionID:   operationID,
		Status:        "pending_approval",
		ApprovalID:    approvalID,
	}
	op := validation.OperationStatusResponse{OperationID: operationID, Status: "pending_approval", TraceID: traceID, Result: resp}
	s.storeOperation(op)
	s.storeIdempotent("apply:"+key, op)
	s.logEvent(traceID, "approval_requested", actorFromRequest(r), map[string]any{"approval_id": approvalID, "operation_id": operationID})
	writeJSON(w, http.StatusAccepted, resp)
}

func (s *server) handleApprovalDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported", traceIDFromContext(r.Context()), nil)
		return
	}
	traceID := traceIDFromContext(r.Context())
	path := strings.TrimPrefix(r.URL.Path, "/v1/approvals/")
	approvalID := strings.TrimSuffix(path, "/decision")
	approvalID = strings.TrimSuffix(approvalID, "/")
	if approvalID == "" || approvalID == path {
		writeError(w, http.StatusBadRequest, "invalid_path", "approval_id is required", traceID, nil)
		return
	}

	var req validation.ApprovalDecisionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), traceID, nil)
		return
	}
	rec, err := s.approval.Decide(approvalID, actorFromRequest(r), req.Decision, req.Reason, time.Now().UTC())
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "approval_not_found", err.Error(), traceID, nil)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_decision", err.Error(), traceID, nil)
		return
	}

	s.logEvent(rec.TraceID, "approval_decided", actorFromRequest(r), map[string]any{"approval_id": approvalID, "status": rec.Status})
	if rec.Status == approval.StatusApproved {
		if err := s.executeApprovedApply(r.Context(), approvalID); err != nil {
			writeError(w, http.StatusInternalServerError, "apply_execution_failed", err.Error(), traceID, nil)
			return
		}
	}

	writeJSON(w, http.StatusOK, validation.ApprovalDecisionResponse{
		ApprovalID: rec.ApprovalID,
		Status:     string(rec.Status),
		DecidedBy:  rec.DecidedBy,
		DecidedAt:  rec.DecidedAt,
	})
}

func (s *server) executeApprovedApply(ctx context.Context, approvalID string) error {
	s.mu.RLock()
	pending, ok := s.pendingApplies[approvalID]
	s.mu.RUnlock()
	if !ok {
		return errors.New("pending apply not found")
	}

	resp, err := s.exec.Apply(ctx, pending.Req)
	if err != nil {
		s.storeOperation(validation.OperationStatusResponse{
			OperationID: pending.OperationID,
			Status:      "failed",
			TraceID:     pending.TraceID,
			Result:      map[string]string{"error": err.Error()},
		})
		s.logEvent(pending.TraceID, "apply_failed", "system", map[string]any{"approval_id": approvalID, "error": err.Error()})
		return err
	}
	s.storeOperation(validation.OperationStatusResponse{
		OperationID: pending.OperationID,
		Status:      resp.Status,
		TraceID:     pending.TraceID,
		Result:      resp,
	})
	s.logEvent(pending.TraceID, "apply_completed", "system", map[string]any{"approval_id": approvalID, "execution_id": resp.ExecutionID})
	return nil
}

func (s *server) handleGetOperation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported", traceIDFromContext(r.Context()), nil)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/operations/")
	op, ok := s.getOperation(id)
	if !ok {
		writeError(w, http.StatusNotFound, "operation_not_found", "operation not found", traceIDFromContext(r.Context()), nil)
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func (s *server) handleGetAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported", traceIDFromContext(r.Context()), nil)
		return
	}
	traceID := strings.TrimPrefix(r.URL.Path, "/v1/audit/")
	writeJSON(w, http.StatusOK, validation.AuditEventsResponse{TraceID: traceID, Events: s.audit.ByTrace(traceID)})
}

func (s *server) logEvent(traceID, eventType, actor string, metadata map[string]any) {
	_ = s.audit.Append(validation.AuditEvent{
		EventID:    newUUID(),
		TraceID:    traceID,
		EventType:  eventType,
		OccurredAt: time.Now().UTC(),
		Actor:      actor,
		Metadata:   metadata,
	})
}

func (s *server) storeIdempotent(key string, status validation.OperationStatusResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idempotency[key] = status
}

func (s *server) getIdempotent(key string) (validation.OperationStatusResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.idempotency[key]
	return v, ok
}

func (s *server) storeOperation(status validation.OperationStatusResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operations[status.OperationID] = status
}

func (s *server) getOperation(id string) (validation.OperationStatusResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.operations[id]
	return v, ok
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, code, message, traceID string, details []validation.ErrorDetail) {
	writeJSON(w, status, validation.ErrorResponse{Code: code, Message: message, TraceID: traceID, Details: details})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type traceContextKey struct{}

func withTraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := strings.TrimSpace(r.Header.Get("X-Trace-Id"))
		if traceID == "" {
			traceID = newUUID()
		}
		r = r.WithContext(context.WithValue(r.Context(), traceContextKey{}, traceID))
		next.ServeHTTP(w, r)
	})
}

func withJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, http.StatusBadRequest, "invalid_content_type", "Content-Type must be application/json", traceIDFromContext(r.Context()), nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func traceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(traceContextKey{}).(string)
	if v == "" {
		return "unknown-trace"
	}
	return v
}

func actorFromRequest(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Actor-Id")); v != "" {
		return v
	}
	return "unknown"
}

func shortID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

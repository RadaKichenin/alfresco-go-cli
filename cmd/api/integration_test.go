package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aborroy/alfresco-cli/internal/audit"
	"github.com/aborroy/alfresco-cli/internal/state"
	"github.com/aborroy/alfresco-cli/internal/validation"
)

type fakeExecutor struct{}

func (f fakeExecutor) Resolve(context.Context, validation.ResolveRequest) (validation.ResolveResponse, error) {
	return validation.ResolveResponse{
		SchemaVersion: validation.SchemaVersion,
		RequestID:     "resolve-test",
		Status:        "not_found",
		Candidates:    []validation.ResolveCandidate{},
		Confidence:    validation.Confidence{Band: "low"},
		NextAction:    "refine_constraints",
	}, nil
}

func (f fakeExecutor) Plan(context.Context, validation.PlanRequest) (validation.PlanResponse, error) {
	return validation.PlanResponse{
		SchemaVersion:    validation.SchemaVersion,
		RequestID:        "plan-test",
		PlanID:           "plan-test-id",
		PlanHash:         "plan-hash-test",
		Status:           "planned",
		Operations:       []validation.PlanOperation{{OpID: "op-1", Description: "test", CLICommand: "noop"}},
		ApprovalRequired: true,
	}, nil
}

func (f fakeExecutor) Apply(context.Context, validation.ApplyRequest) (validation.ApplyResponse, error) {
	return validation.ApplyResponse{
		SchemaVersion: validation.SchemaVersion,
		RequestID:     "apply-test",
		ExecutionID:   "exec-test",
		Status:        "succeeded",
		Results:       []validation.ApplyResult{{OpID: "op-1", ExitCode: 0, Stdout: "ok", Stderr: ""}},
	}, nil
}

func TestBlackBoxFlow_WithSQLitePersistence(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gateway.db")
	auditPath := filepath.Join(tmp, "audit.log")

	srv1, close1 := newTestServer(t, dbPath, auditPath)
	defer close1()

	planBody := map[string]any{
		"schema_version":     "1.0",
		"resolve_request_id": "resolve-1",
		"action":             "upload_new_version",
		"selection": map[string]any{
			"target_node_id": "11111111-1111-4111-8111-111111111111",
		},
		"payload": map[string]any{
			"local_file_path": "/tmp/file.pdf",
		},
		"safety": map[string]any{
			"dry_run":               true,
			"require_preconditions": false,
		},
	}
	respPlan := mustPOST(t, srv1.URL+"/v1/plan", planBody, map[string]string{"Idempotency-Key": "plan-key-1", "X-Actor-Id": "operator-1"})
	if respPlan.StatusCode != http.StatusOK {
		t.Fatalf("plan status = %d", respPlan.StatusCode)
	}
	var plan validation.PlanResponse
	decodeResp(t, respPlan, &plan)
	if plan.PlanID == "" || plan.PlanHash == "" {
		t.Fatalf("expected non-empty plan id/hash")
	}

	applyBody := map[string]any{
		"schema_version": "1.0",
		"plan_id":        plan.PlanID,
		"plan_hash":      plan.PlanHash,
	}
	respApply := mustPOST(t, srv1.URL+"/v1/apply", applyBody, map[string]string{"Idempotency-Key": "apply-key-1", "X-Actor-Id": "operator-1"})
	if respApply.StatusCode != http.StatusAccepted {
		t.Fatalf("apply status = %d", respApply.StatusCode)
	}
	var apply validation.ApplyResponse
	decodeResp(t, respApply, &apply)
	if apply.ApprovalID == "" || apply.ExecutionID == "" {
		t.Fatalf("expected approval_id and execution_id")
	}

	close1()

	srv2, close2 := newTestServer(t, dbPath, auditPath)
	defer close2()

	approvalGet, err := http.Get(srv2.URL + "/v1/approvals/" + apply.ApprovalID)
	if err != nil {
		t.Fatalf("get approval failed: %v", err)
	}
	if approvalGet.StatusCode != http.StatusOK {
		t.Fatalf("approval get status=%d", approvalGet.StatusCode)
	}
	var approval validation.ApprovalStatusResponse
	decodeResp(t, approvalGet, &approval)
	if approval.Status != "pending" {
		t.Fatalf("expected pending status, got %s", approval.Status)
	}

	listResp, err := http.Get(srv2.URL + "/v1/approvals?status=pending&limit=10")
	if err != nil {
		t.Fatalf("list approvals failed: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list approvals status=%d", listResp.StatusCode)
	}
	var list validation.ApprovalListResponse
	decodeResp(t, listResp, &list)
	if list.Count < 1 {
		t.Fatalf("expected at least one pending approval")
	}

	decisionBody := map[string]any{"decision": "approve", "reason": "ok"}
	respDecision := mustPOST(t, srv2.URL+"/v1/approvals/"+apply.ApprovalID+"/decision", decisionBody, map[string]string{"X-Actor-Id": "approver-1"})
	if respDecision.StatusCode != http.StatusOK {
		t.Fatalf("approval decision status = %d", respDecision.StatusCode)
	}

	opResp, err := http.Get(srv2.URL + "/v1/operations/" + apply.ExecutionID)
	if err != nil {
		t.Fatalf("get operation failed: %v", err)
	}
	if opResp.StatusCode != http.StatusOK {
		t.Fatalf("operation status=%d", opResp.StatusCode)
	}
	var op validation.OperationStatusResponse
	decodeResp(t, opResp, &op)
	if op.Status != "succeeded" {
		t.Fatalf("expected succeeded operation, got %s", op.Status)
	}
}

func newTestServer(t *testing.T, dbPath, auditPath string) (*httptest.Server, func()) {
	t.Helper()
	st, err := state.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("state init failed: %v", err)
	}
	s := &server{
		exec:         fakeExecutor{},
		state:        st,
		audit:        audit.NewLogger(auditPath),
		authRequired: false,
	}
	h := buildHandler(s)
	hts := httptest.NewServer(h)
	cleanup := func() {
		hts.Close()
		_ = st.Close()
	}
	return hts, cleanup
}

func mustPOST(t *testing.T, url string, body any, headers map[string]string) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("request create failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	return resp
}

func decodeResp(t *testing.T, r *http.Response, dst any) {
	t.Helper()
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

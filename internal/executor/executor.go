package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/aborroy/alfresco-cli/internal/validation"
)

type Executor interface {
	Resolve(context.Context, validation.ResolveRequest) (validation.ResolveResponse, error)
	Plan(context.Context, validation.PlanRequest) (validation.PlanResponse, error)
	Apply(context.Context, validation.ApplyRequest) (validation.ApplyResponse, error)
}

type CLIExecutor struct {
	AlfrescoAgentBin string
}

func NewCLIExecutor(agentBin string) *CLIExecutor {
	return &CLIExecutor{AlfrescoAgentBin: agentBin}
}

func (e *CLIExecutor) Resolve(_ context.Context, req validation.ResolveRequest) (validation.ResolveResponse, error) {
	args := []string{
		"resolve",
		"--operation", req.Operation,
		"--kind", req.Target.Kind,
		"--name", req.Target.Name,
		"--site-id", req.Scope.SiteID,
		"--root-node-id", req.Scope.RootNodeID,
		"--max-depth", strconv.Itoa(req.Scope.MaxDepth),
		"--expected-site-id", fallback(req.Target.ExpectedSiteID, req.Scope.SiteID),
		"--query-text", req.Target.QueryText,
		"--modified-within-days", strconv.Itoa(req.Target.ModifiedWithinDays),
		"--max-candidates", strconv.Itoa(req.Policy.MaxCandidates),
		"--require-unique", strconv.FormatBool(req.Policy.RequireUnique),
	}
	if req.Target.Extension != "" {
		args = append(args, "--extension", req.Target.Extension)
	}
	if req.Target.ExpectedParentPath != "" {
		args = append(args, "--expected-parent-path", req.Target.ExpectedParentPath)
	}
	if len(req.Target.AncestorNames) > 0 {
		args = append(args, "--ancestor-names", strings.Join(req.Target.AncestorNames, ","))
	}

	stdout, stderr, err := e.run(args...)
	if err != nil {
		return validation.ResolveResponse{}, fmt.Errorf("resolve command failed: %w; stderr=%s", err, stderr)
	}
	var resp validation.ResolveResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		return validation.ResolveResponse{}, fmt.Errorf("failed to parse resolve JSON: %w; stdout=%s", err, stdout)
	}
	return resp, nil
}

func (e *CLIExecutor) Plan(_ context.Context, req validation.PlanRequest) (validation.PlanResponse, error) {
	args := []string{
		"plan",
		"--resolve-request-id", req.ResolveRequestID,
		"--action", req.Action,
		"--dry-run", strconv.FormatBool(req.Safety.DryRun),
		"--require-preconditions", strconv.FormatBool(req.Safety.RequirePreconditions),
	}
	if req.Selection.TargetNodeID != "" {
		args = append(args, "--target-node-id", req.Selection.TargetNodeID)
	}
	if req.Selection.TargetParentNodeID != "" {
		args = append(args, "--target-parent-node-id", req.Selection.TargetParentNodeID)
	}
	if req.Payload.LocalFilePath != "" {
		args = append(args, "--local-file-path", req.Payload.LocalFilePath)
	}
	if req.Payload.NewName != "" {
		args = append(args, "--new-name", req.Payload.NewName)
	}
	if len(req.Payload.Properties) > 0 {
		raw, _ := json.Marshal(req.Payload.Properties)
		args = append(args, "--properties-json", string(raw))
	}
	if req.Safety.ExpectedModifiedAt != "" {
		args = append(args, "--expected-modified-at", req.Safety.ExpectedModifiedAt)
	}
	if req.Safety.ExpectedChecksum != "" {
		args = append(args, "--expected-checksum", req.Safety.ExpectedChecksum)
	}

	stdout, stderr, err := e.run(args...)
	if err != nil {
		return validation.PlanResponse{}, fmt.Errorf("plan command failed: %w; stderr=%s", err, stderr)
	}
	var resp validation.PlanResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		return validation.PlanResponse{}, fmt.Errorf("failed to parse plan JSON: %w; stdout=%s", err, stdout)
	}
	if resp.Status == "ready" || resp.Status == "needs_confirmation" {
		resp.Status = "planned"
	}
	return resp, nil
}

func (e *CLIExecutor) Apply(_ context.Context, req validation.ApplyRequest) (validation.ApplyResponse, error) {
	args := []string{
		"apply",
		"--plan-id", req.PlanID,
		"--plan-hash", req.PlanHash,
		"--approved", strconv.FormatBool(req.Approved),
	}
	if req.IdempotencyKey != "" {
		args = append(args, "--idempotency-key", req.IdempotencyKey)
	}
	if req.Runtime.HTTPTimeout != "" {
		args = append(args, "--http-timeout", req.Runtime.HTTPTimeout)
	}
	if req.Runtime.HTTPRetries >= 0 {
		args = append(args, "--http-retries", strconv.Itoa(req.Runtime.HTTPRetries))
	}
	if req.Runtime.HTTPRetryWait != "" {
		args = append(args, "--http-retry-wait", req.Runtime.HTTPRetryWait)
	}

	stdout, stderr, err := e.run(args...)
	if err != nil {
		return validation.ApplyResponse{}, fmt.Errorf("apply command failed: %w; stderr=%s", err, stderr)
	}
	var resp validation.ApplyResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		return validation.ApplyResponse{}, fmt.Errorf("failed to parse apply JSON: %w; stdout=%s", err, stdout)
	}
	return resp, nil
}

func (e *CLIExecutor) run(args ...string) (string, string, error) {
	cmd := exec.Command(e.AlfrescoAgentBin, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), fmt.Errorf("exit code %d", exitErr.ExitCode())
		}
		return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), nil
}

func fallback(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

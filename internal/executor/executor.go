package executor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

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
	// Placeholder implementation. Replace with real subprocess invocation of:
	// <alfresco-agent-bin> resolve --request-json ...
	resp := validation.ResolveResponse{
		SchemaVersion: validation.SchemaVersion,
		RequestID:     "resolve-" + newID(),
		Status:        "not_found",
		Candidates:    []validation.ResolveCandidate{},
		Confidence: validation.Confidence{
			TopScore: 0, SecondScore: 0, Delta: 0, Band: "low",
		},
		NextAction: "refine_constraints",
		Questions:  []string{"No candidates found. Add expected_parent_path or ancestor_names."},
	}
	if req.Target.ExpectedSiteID != "" && req.Target.ExpectedSiteID != req.Scope.SiteID {
		resp.Status = "out_of_scope"
		resp.NextAction = "fix_scope"
		resp.Questions = []string{"expected_site_id does not match scope.site_id"}
	}
	return resp, nil
}

func (e *CLIExecutor) Plan(_ context.Context, req validation.PlanRequest) (validation.PlanResponse, error) {
	return validation.PlanResponse{
		SchemaVersion:    validation.SchemaVersion,
		RequestID:        "plan-" + newID(),
		PlanID:           newUUIDLike(),
		PlanHash:         fmt.Sprintf("sha256:%s", newID()),
		Status:           "planned",
		ApprovalRequired: true,
		Operations: []validation.PlanOperation{
			{
				OpID:        "op-1",
				Description: "Execute action through alfresco-go-cli",
				CLICommand:  buildPlannedCommand(req),
			},
		},
		Preconditions: []string{"Resolve status must be resolved"},
		Postconditions: []string{
			"Target node state matches expected operation",
		},
	}, nil
}

func (e *CLIExecutor) Apply(_ context.Context, _ validation.ApplyRequest) (validation.ApplyResponse, error) {
	return validation.ApplyResponse{
		SchemaVersion: validation.SchemaVersion,
		RequestID:     "apply-" + newID(),
		ExecutionID:   newUUIDLike(),
		Status:        "succeeded",
		Results: []validation.ApplyResult{
			{OpID: "op-1", ExitCode: 0, Stdout: "stub execution complete", Stderr: ""},
		},
	}, nil
}

func buildPlannedCommand(req validation.PlanRequest) string {
	switch req.Action {
	case "upload_new_version":
		return "alfresco node update -i <target_node_id> -f <local_file_path> -o json"
	case "update_metadata":
		return "alfresco node update -i <target_node_id> -p key=value -o json"
	case "create_child":
		return "alfresco node create -i <target_parent_node_id> -n <new_name> -f <local_file_path> -o json"
	default:
		return "alfresco <unsupported-action>"
	}
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func newUUIDLike() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("00000000-0000-0000-0000-%012d", time.Now().Unix()%1000000000000)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16],
	)
}

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const schemaVersion = "1.0"

type ResolveRequest struct {
	SchemaVersion string `json:"schema_version"`
	RequestID     string `json:"request_id"`
	Operation     string `json:"operation"`
	Scope         struct {
		AllowedRoots []struct {
			SiteID     string `json:"site_id"`
			RootNodeID string `json:"root_node_id"`
			MaxDepth   int    `json:"max_depth"`
		} `json:"allowed_roots"`
	} `json:"scope"`
	Target struct {
		Kind               string            `json:"kind"`
		Name               string            `json:"name"`
		Extension          string            `json:"extension"`
		ExpectedSiteID     string            `json:"expected_site_id"`
		ExpectedParentPath string            `json:"expected_parent_path"`
		ExpectedMIMEType   string            `json:"expected_mime_type"`
		Metadata           map[string]string `json:"metadata"`
	} `json:"target"`
	Hints struct {
		AncestorNames      []string `json:"ancestor_names"`
		CreatedBy          []string `json:"created_by"`
		ModifiedWithinDays int      `json:"modified_within_days"`
		QueryText          string   `json:"query_text"`
	} `json:"hints"`
	Policy struct {
		MaxCandidates      int  `json:"max_candidates"`
		RequireUnique      bool `json:"require_unique"`
		ReturnExplanations bool `json:"return_explanations"`
	} `json:"policy"`
}

type ResolveResponse struct {
	SchemaVersion       string             `json:"schema_version"`
	RequestID           string             `json:"request_id"`
	Status              string             `json:"status"`
	BestCandidateNodeID string             `json:"best_candidate_node_id,omitempty"`
	Candidates          []ResolveCandidate `json:"candidates"`
	Confidence          ResolveConfidence  `json:"confidence"`
	NextAction          string             `json:"next_action"`
	Questions           []string           `json:"questions,omitempty"`
	Error               string             `json:"error,omitempty"`
}

type ResolveCandidate struct {
	NodeID         string         `json:"node_id"`
	SiteID         string         `json:"site_id"`
	FullPath       string         `json:"full_path"`
	Kind           string         `json:"kind"`
	Score          float64        `json:"score"`
	ScoreBreakdown ScoreBreakdown `json:"score_breakdown"`
	Reasons        []string       `json:"reasons,omitempty"`
	modifiedAt     string
	createdBy      string
	extension      string
	normalizedName string
}

type ScoreBreakdown struct {
	Path     float64 `json:"path"`
	Name     float64 `json:"name"`
	TypeMeta float64 `json:"type_meta"`
	Semantic float64 `json:"semantic"`
	Recency  float64 `json:"recency"`
	History  float64 `json:"history"`
	Penalty  float64 `json:"penalty"`
}

type ResolveConfidence struct {
	TopScore    float64 `json:"top_score"`
	SecondScore float64 `json:"second_score"`
	Delta       float64 `json:"delta"`
	Band        string  `json:"band"`
}

type PlanRequest struct {
	SchemaVersion    string `json:"schema_version"`
	RequestID        string `json:"request_id"`
	ResolveRequestID string `json:"resolve_request_id"`
	Action           string `json:"action"`
	Selection        struct {
		TargetNodeID       string `json:"target_node_id"`
		TargetParentNodeID string `json:"target_parent_node_id"`
	} `json:"selection"`
	Payload struct {
		LocalFilePath string            `json:"local_file_path"`
		NewName       string            `json:"new_name"`
		Properties    map[string]string `json:"properties"`
	} `json:"payload"`
	Safety struct {
		DryRun               bool   `json:"dry_run"`
		RequirePreconditions bool   `json:"require_preconditions"`
		ExpectedModifiedAt   string `json:"expected_modified_at"`
		ExpectedChecksum     string `json:"expected_checksum"`
	} `json:"safety"`
}

type PlanResponse struct {
	SchemaVersion    string          `json:"schema_version"`
	RequestID        string          `json:"request_id"`
	PlanID           string          `json:"plan_id"`
	PlanHash         string          `json:"plan_hash"`
	Status           string          `json:"status"`
	Operations       []PlanOperation `json:"operations"`
	Preconditions    []string        `json:"preconditions,omitempty"`
	Postconditions   []string        `json:"postconditions,omitempty"`
	ApprovalRequired bool            `json:"approval_required"`
	Error            string          `json:"error,omitempty"`
}

type PlanOperation struct {
	OpID        string   `json:"op_id"`
	Description string   `json:"description"`
	CLICommand  string   `json:"cli_command"`
	Args        []string `json:"args,omitempty"`
}

type storedPlan struct {
	PlanResponse PlanResponse `json:"plan_response"`
}

type ApplyRequest struct {
	SchemaVersion  string `json:"schema_version"`
	RequestID      string `json:"request_id"`
	PlanID         string `json:"plan_id"`
	PlanHash       string `json:"plan_hash"`
	IdempotencyKey string `json:"idempotency_key"`
	Approved       bool   `json:"approved"`
	Runtime        struct {
		HTTPTimeout   string `json:"http_timeout"`
		HTTPRetries   int    `json:"http_retries"`
		HTTPRetryWait string `json:"http_retry_wait"`
	} `json:"runtime"`
}

type ApplyResponse struct {
	SchemaVersion string         `json:"schema_version"`
	RequestID     string         `json:"request_id"`
	ExecutionID   string         `json:"execution_id"`
	Status        string         `json:"status"`
	Results       []ApplyResult  `json:"results"`
	Artifacts     map[string]any `json:"artifacts,omitempty"`
	Error         string         `json:"error,omitempty"`
}

type ApplyResult struct {
	OpID     string `json:"op_id"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type nodeListResponse struct {
	List struct {
		Pagination struct {
			Count        int  `json:"count"`
			HasMoreItems bool `json:"hasMoreItems"`
			SkipCount    int  `json:"skipCount"`
		} `json:"pagination"`
		Entries []struct {
			Entry nodeEntry `json:"entry"`
		} `json:"entries"`
	} `json:"list"`
}

type nodeEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	NodeType      string `json:"nodeType"`
	IsFolder      bool   `json:"isFolder"`
	IsFile        bool   `json:"isFile"`
	ModifiedAt    string `json:"modifiedAt"`
	CreatedByUser struct {
		ID string `json:"id"`
	} `json:"createdByUser"`
	ModifiedByUser struct {
		ID string `json:"id"`
	} `json:"modifiedByUser"`
}

type traversalNode struct {
	SiteID string
	Path   string
	Depth  int
	Entry  nodeEntry
}

func main() {
	rand.Seed(time.Now().UnixNano())
	if len(os.Args) < 2 {
		printErrorAndExit("expected subcommand: resolve, plan, apply")
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "resolve":
		handleResolve(os.Args[2:])
	case "plan":
		handlePlan(os.Args[2:])
	case "apply":
		handleApply(os.Args[2:])
	default:
		printErrorAndExit("unsupported subcommand: " + subcommand)
	}
}

func handleResolve(args []string) {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	var requestFile string
	var requestJSON string
	var requestID string
	var operation string
	var kind string
	var name string
	var extension string
	var siteID string
	var rootNodeID string
	var maxDepth int
	var expectedParentPath string
	var expectedSiteID string
	var queryText string
	var ancestorCSV string
	var modifiedWithinDays int
	var maxCandidates int
	var requireUniqueRaw string

	fs.StringVar(&requestFile, "request-file", "", "Path to resolve request JSON")
	fs.StringVar(&requestJSON, "request-json", "", "Inline resolve request JSON")
	fs.StringVar(&requestID, "request-id", "", "Request ID")
	fs.StringVar(&operation, "operation", "locate_for_upload", "Operation")
	fs.StringVar(&kind, "kind", "", "Target kind: file or folder")
	fs.StringVar(&name, "name", "", "Target name")
	fs.StringVar(&extension, "extension", "", "Expected file extension")
	fs.StringVar(&siteID, "site-id", "", "Site identifier")
	fs.StringVar(&rootNodeID, "root-node-id", "", "Root node ID")
	fs.IntVar(&maxDepth, "max-depth", 4, "Traversal max depth")
	fs.StringVar(&expectedParentPath, "expected-parent-path", "", "Expected parent path")
	fs.StringVar(&expectedSiteID, "expected-site-id", "", "Expected site ID")
	fs.StringVar(&queryText, "query-text", "", "Intent text")
	fs.StringVar(&ancestorCSV, "ancestor-names", "", "Comma separated expected ancestor names")
	fs.IntVar(&modifiedWithinDays, "modified-within-days", 0, "Recency hint")
	fs.IntVar(&maxCandidates, "max-candidates", 5, "Max candidates")
	fs.StringVar(&requireUniqueRaw, "require-unique", "", "Require unique high confidence winner (true/false)")
	_ = fs.Parse(args)

	req, err := loadResolveRequest(requestFile, requestJSON)
	if err != nil {
		req = ResolveRequest{}
	}
	if req.SchemaVersion == "" {
		req.SchemaVersion = schemaVersion
	}
	if req.RequestID == "" {
		req.RequestID = fallbackID(requestID, "resolve")
	}
	if req.Operation == "" {
		req.Operation = operation
	}
	if req.Target.Kind == "" {
		req.Target.Kind = kind
	}
	if req.Target.Name == "" {
		req.Target.Name = name
	}
	if req.Target.Extension == "" {
		req.Target.Extension = extension
	}
	if req.Target.ExpectedParentPath == "" {
		req.Target.ExpectedParentPath = expectedParentPath
	}
	if req.Target.ExpectedSiteID == "" {
		req.Target.ExpectedSiteID = expectedSiteID
	}
	if req.Hints.QueryText == "" {
		req.Hints.QueryText = queryText
	}
	if len(req.Hints.AncestorNames) == 0 && strings.TrimSpace(ancestorCSV) != "" {
		req.Hints.AncestorNames = splitCSV(ancestorCSV)
	}
	if req.Hints.ModifiedWithinDays == 0 {
		req.Hints.ModifiedWithinDays = modifiedWithinDays
	}
	if req.Policy.MaxCandidates == 0 {
		req.Policy.MaxCandidates = maxCandidates
	}
	req.Policy.RequireUnique = mergeBool(req.Policy.RequireUnique, true, requireUniqueRaw)
	if len(req.Scope.AllowedRoots) == 0 && rootNodeID != "" {
		req.Scope.AllowedRoots = []struct {
			SiteID     string `json:"site_id"`
			RootNodeID string `json:"root_node_id"`
			MaxDepth   int    `json:"max_depth"`
		}{{SiteID: siteID, RootNodeID: rootNodeID, MaxDepth: maxDepth}}
	}

	resp := resolve(req)
	printJSON(resp)
}

func handlePlan(args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	var requestFile string
	var requestJSON string
	var requestID string
	var resolveRequestID string
	var action string
	var targetNodeID string
	var targetParentNodeID string
	var localFilePath string
	var newName string
	var propertiesJSON string
	var dryRunRaw string
	var requirePreconditionsRaw string
	var expectedModifiedAt string
	var expectedChecksum string

	fs.StringVar(&requestFile, "request-file", "", "Path to plan request JSON")
	fs.StringVar(&requestJSON, "request-json", "", "Inline plan request JSON")
	fs.StringVar(&requestID, "request-id", "", "Request ID")
	fs.StringVar(&resolveRequestID, "resolve-request-id", "", "Resolve request ID")
	fs.StringVar(&action, "action", "", "Plan action")
	fs.StringVar(&targetNodeID, "target-node-id", "", "Target node ID")
	fs.StringVar(&targetParentNodeID, "target-parent-node-id", "", "Target parent node ID")
	fs.StringVar(&localFilePath, "local-file-path", "", "Local file path")
	fs.StringVar(&newName, "new-name", "", "New name")
	fs.StringVar(&propertiesJSON, "properties-json", "", "Properties JSON object")
	fs.StringVar(&dryRunRaw, "dry-run", "", "Dry run (true/false)")
	fs.StringVar(&requirePreconditionsRaw, "require-preconditions", "", "Require preconditions (true/false)")
	fs.StringVar(&expectedModifiedAt, "expected-modified-at", "", "Expected modified timestamp")
	fs.StringVar(&expectedChecksum, "expected-checksum", "", "Expected checksum")
	_ = fs.Parse(args)

	req, err := loadPlanRequest(requestFile, requestJSON)
	if err != nil {
		req = PlanRequest{}
	}
	if req.SchemaVersion == "" {
		req.SchemaVersion = schemaVersion
	}
	if req.RequestID == "" {
		req.RequestID = fallbackID(requestID, "plan")
	}
	if req.ResolveRequestID == "" {
		req.ResolveRequestID = resolveRequestID
	}
	if req.Action == "" {
		req.Action = action
	}
	if req.Selection.TargetNodeID == "" {
		req.Selection.TargetNodeID = targetNodeID
	}
	if req.Selection.TargetParentNodeID == "" {
		req.Selection.TargetParentNodeID = targetParentNodeID
	}
	if req.Payload.LocalFilePath == "" {
		req.Payload.LocalFilePath = localFilePath
	}
	if req.Payload.NewName == "" {
		req.Payload.NewName = newName
	}
	if len(req.Payload.Properties) == 0 && strings.TrimSpace(propertiesJSON) != "" {
		_ = json.Unmarshal([]byte(propertiesJSON), &req.Payload.Properties)
	}
	req.Safety.DryRun = mergeBool(req.Safety.DryRun, true, dryRunRaw)
	req.Safety.RequirePreconditions = mergeBool(req.Safety.RequirePreconditions, true, requirePreconditionsRaw)
	if req.Safety.ExpectedModifiedAt == "" {
		req.Safety.ExpectedModifiedAt = expectedModifiedAt
	}
	if req.Safety.ExpectedChecksum == "" {
		req.Safety.ExpectedChecksum = expectedChecksum
	}

	resp := plan(req)
	printJSON(resp)
}

func handleApply(args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	var requestFile string
	var requestJSON string
	var requestID string
	var planID string
	var planHash string
	var idempotencyKey string
	var approvedRaw string
	var httpTimeout string
	var httpRetries int
	var httpRetryWait string

	fs.StringVar(&requestFile, "request-file", "", "Path to apply request JSON")
	fs.StringVar(&requestJSON, "request-json", "", "Inline apply request JSON")
	fs.StringVar(&requestID, "request-id", "", "Request ID")
	fs.StringVar(&planID, "plan-id", "", "Plan ID")
	fs.StringVar(&planHash, "plan-hash", "", "Plan hash")
	fs.StringVar(&idempotencyKey, "idempotency-key", "", "Idempotency key")
	fs.StringVar(&approvedRaw, "approved", "", "Approved execution (true/false)")
	fs.StringVar(&httpTimeout, "http-timeout", "", "HTTP timeout override")
	fs.IntVar(&httpRetries, "http-retries", -1, "HTTP retries override")
	fs.StringVar(&httpRetryWait, "http-retry-wait", "", "HTTP retry wait override")
	_ = fs.Parse(args)

	req, err := loadApplyRequest(requestFile, requestJSON)
	if err != nil {
		req = ApplyRequest{}
	}
	if req.SchemaVersion == "" {
		req.SchemaVersion = schemaVersion
	}
	if req.RequestID == "" {
		req.RequestID = fallbackID(requestID, "apply")
	}
	if req.PlanID == "" {
		req.PlanID = planID
	}
	if req.PlanHash == "" {
		req.PlanHash = planHash
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = idempotencyKey
	}
	req.Approved = mergeBool(req.Approved, false, approvedRaw)
	if req.Runtime.HTTPTimeout == "" {
		req.Runtime.HTTPTimeout = httpTimeout
	}
	if req.Runtime.HTTPRetries == 0 && httpRetries >= 0 {
		req.Runtime.HTTPRetries = httpRetries
	}
	if req.Runtime.HTTPRetryWait == "" {
		req.Runtime.HTTPRetryWait = httpRetryWait
	}

	resp := apply(req)
	printJSON(resp)
}

func resolve(req ResolveRequest) ResolveResponse {
	resp := ResolveResponse{
		SchemaVersion: schemaVersion,
		RequestID:     req.RequestID,
		Candidates:    []ResolveCandidate{},
		Confidence: ResolveConfidence{
			Band: "low",
		},
		Status:     "not_found",
		NextAction: "refine_constraints",
	}

	if req.Target.Name == "" {
		resp.Error = "target.name is required"
		resp.Status = "not_found"
		return resp
	}
	if len(req.Scope.AllowedRoots) == 0 {
		resp.Status = "out_of_scope"
		resp.NextAction = "abort"
		resp.Error = "scope.allowed_roots is required"
		return resp
	}

	maxCandidates := req.Policy.MaxCandidates
	if maxCandidates <= 0 || maxCandidates > 20 {
		maxCandidates = 5
	}

	candidates := []ResolveCandidate{}
	for _, root := range req.Scope.AllowedRoots {
		depthLimit := root.MaxDepth
		if depthLimit <= 0 {
			depthLimit = 4
		}
		start := traversalNode{
			SiteID: root.SiteID,
			Path:   "/" + strings.Trim(strings.TrimSpace(root.SiteID), "/"),
			Depth:  0,
			Entry:  nodeEntry{ID: root.RootNodeID, Name: root.RootNodeID, IsFolder: true},
		}
		if start.Path == "/" {
			start.Path = "/root"
		}

		queue := []traversalNode{start}
		visited := map[string]bool{}

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if visited[current.Entry.ID] {
				continue
			}
			visited[current.Entry.ID] = true

			if current.Depth > depthLimit {
				continue
			}

			children, err := fetchAllChildren(current.Entry.ID)
			if err != nil {
				continue
			}

			for _, child := range children {
				path := strings.TrimRight(current.Path, "/") + "/" + child.Name
				if matchesKind(req.Target.Kind, child.IsFile, child.IsFolder) {
					candidate := buildCandidate(req, root.SiteID, path, child)
					candidates = append(candidates, candidate)
				}
				if child.IsFolder && current.Depth+1 <= depthLimit {
					queue = append(queue, traversalNode{
						SiteID: root.SiteID,
						Path:   path,
						Depth:  current.Depth + 1,
						Entry:  child,
					})
				}
			}
		}
	}

	if len(candidates) == 0 {
		resp.Status = "not_found"
		resp.NextAction = "refine_constraints"
		resp.Questions = []string{"No candidates found. Add site, expected path, or ancestor names."}
		return resp
	}

	applyDuplicatePenalties(candidates)

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}

	resp.Candidates = candidates
	resp.Confidence.TopScore = candidates[0].Score
	if len(candidates) > 1 {
		resp.Confidence.SecondScore = candidates[1].Score
	}
	resp.Confidence.Delta = resp.Confidence.TopScore - resp.Confidence.SecondScore
	resp.Confidence.Band = confidenceBand(resp.Confidence.TopScore, resp.Confidence.Delta)

	high := resp.Confidence.TopScore >= 85 && resp.Confidence.Delta >= 15
	medium := resp.Confidence.TopScore >= 70 && resp.Confidence.Delta >= 8

	if req.Policy.RequireUnique {
		if high {
			resp.Status = "resolved"
			resp.NextAction = "auto_plan"
			resp.BestCandidateNodeID = candidates[0].NodeID
			return resp
		}
		resp.Status = "ambiguous"
		resp.NextAction = "ask_user_disambiguation"
		resp.Questions = []string{"Multiple similar locations found. Confirm site/path before writing."}
		return resp
	}

	if high || medium {
		resp.Status = "resolved"
		resp.NextAction = "auto_plan"
		resp.BestCandidateNodeID = candidates[0].NodeID
		return resp
	}

	resp.Status = "ambiguous"
	resp.NextAction = "ask_user_disambiguation"
	resp.Questions = []string{"Low confidence match. Provide expected parent path or site."}
	return resp
}

func plan(req PlanRequest) PlanResponse {
	resp := PlanResponse{
		SchemaVersion:    schemaVersion,
		RequestID:        req.RequestID,
		PlanID:           newID("plan"),
		Status:           "blocked",
		Operations:       []PlanOperation{},
		ApprovalRequired: true,
	}

	if req.Action == "" {
		resp.Error = "action is required"
		return resp
	}

	buildErr := errors.New("")
	switch req.Action {
	case "upload_new_version":
		if req.Selection.TargetNodeID == "" {
			buildErr = errors.New("selection.target_node_id is required")
			break
		}
		if req.Payload.LocalFilePath == "" {
			buildErr = errors.New("payload.local_file_path is required")
			break
		}
		resp.Operations = append(resp.Operations, PlanOperation{
			OpID:        "op-upload-1",
			Description: "Upload a new file version to an existing node",
			Args: []string{
				"node", "update", "-i", req.Selection.TargetNodeID,
				"-f", req.Payload.LocalFilePath,
				"--format", "json",
			},
		})
		resp.Postconditions = append(resp.Postconditions, "Target node content is updated")
	case "update_metadata":
		if req.Selection.TargetNodeID == "" {
			buildErr = errors.New("selection.target_node_id is required")
			break
		}
		args := []string{"node", "update", "-i", req.Selection.TargetNodeID}
		if req.Payload.NewName != "" {
			args = append(args, "-n", req.Payload.NewName)
		}
		if len(req.Payload.Properties) > 0 {
			keys := make([]string, 0, len(req.Payload.Properties))
			for key := range req.Payload.Properties {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				args = append(args, "--properties", key+"="+req.Payload.Properties[key])
			}
		}
		if len(args) == 4 {
			buildErr = errors.New("update_metadata requires payload.new_name or payload.properties")
			break
		}
		args = append(args, "--format", "json")
		resp.Operations = append(resp.Operations, PlanOperation{
			OpID:        "op-meta-1",
			Description: "Update node metadata",
			Args:        args,
		})
		resp.Postconditions = append(resp.Postconditions, "Node metadata reflects requested fields")
	case "create_child":
		if req.Selection.TargetParentNodeID == "" {
			buildErr = errors.New("selection.target_parent_node_id is required")
			break
		}
		if req.Payload.NewName == "" {
			buildErr = errors.New("payload.new_name is required")
			break
		}
		nodeType := "cm:folder"
		args := []string{
			"node", "create", "-i", req.Selection.TargetParentNodeID,
			"-n", req.Payload.NewName,
		}
		if req.Payload.LocalFilePath != "" {
			nodeType = "cm:content"
		}
		args = append(args, "-t", nodeType)
		if req.Payload.LocalFilePath != "" {
			args = append(args, "-f", req.Payload.LocalFilePath)
		}
		args = append(args, "--format", "json")
		resp.Operations = append(resp.Operations, PlanOperation{
			OpID:        "op-create-1",
			Description: "Create child node under selected parent",
			Args:        args,
		})
		resp.Postconditions = append(resp.Postconditions, "New child node is created under parent")
	default:
		buildErr = fmt.Errorf("unsupported action: %s", req.Action)
	}

	if buildErr != nil && buildErr.Error() != "" {
		resp.Error = buildErr.Error()
		return resp
	}

	for i := range resp.Operations {
		resp.Operations[i].CLICommand = shellJoin(append([]string{alfrescoBinary()}, resp.Operations[i].Args...))
	}

	if req.Safety.RequirePreconditions {
		if req.Safety.ExpectedModifiedAt != "" {
			resp.Preconditions = append(resp.Preconditions, "modifiedAt must equal "+req.Safety.ExpectedModifiedAt)
		}
		if req.Safety.ExpectedChecksum != "" {
			resp.Preconditions = append(resp.Preconditions, "checksum must equal "+req.Safety.ExpectedChecksum)
		}
	}

	hashPayload := struct {
		Action     string          `json:"action"`
		Selection  any             `json:"selection"`
		Payload    any             `json:"payload"`
		Safety     any             `json:"safety"`
		Operations []PlanOperation `json:"operations"`
	}{
		Action:     req.Action,
		Selection:  req.Selection,
		Payload:    req.Payload,
		Safety:     req.Safety,
		Operations: resp.Operations,
	}
	raw, _ := json.Marshal(hashPayload)
	sum := sha256.Sum256(raw)
	resp.PlanHash = hex.EncodeToString(sum[:])
	if req.Safety.DryRun {
		resp.Status = "needs_confirmation"
	} else {
		resp.Status = "ready"
	}

	if err := savePlan(resp); err != nil {
		resp.Status = "blocked"
		resp.Error = "failed to persist plan: " + err.Error()
	}

	return resp
}

func apply(req ApplyRequest) ApplyResponse {
	resp := ApplyResponse{
		SchemaVersion: schemaVersion,
		RequestID:     req.RequestID,
		ExecutionID:   newID("exec"),
		Status:        "aborted",
		Results:       []ApplyResult{},
	}

	if req.PlanID == "" {
		resp.Error = "plan_id is required"
		return resp
	}
	if req.PlanHash == "" {
		resp.Error = "plan_hash is required"
		return resp
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = newID("idem")
	}

	plan, err := loadPlan(req.PlanID)
	if err != nil {
		resp.Status = "failed"
		resp.Error = "failed to load plan: " + err.Error()
		return resp
	}
	if plan.PlanHash != req.PlanHash {
		resp.Status = "precondition_failed"
		resp.Error = "plan hash mismatch"
		return resp
	}

	if plan.Status == "needs_confirmation" && !req.Approved {
		resp.Status = "aborted"
		resp.Error = "plan requires explicit approval"
		return resp
	}
	if !req.Approved {
		resp.Status = "aborted"
		resp.Error = "approved=false"
		return resp
	}

	if alreadyApplied(req.IdempotencyKey) {
		resp.Status = "succeeded"
		resp.Artifacts = map[string]any{"idempotent": true}
		return resp
	}

	for _, op := range plan.Operations {
		args := append([]string{}, op.Args...)
		if len(args) == 0 {
			resp.Results = append(resp.Results, ApplyResult{
				OpID:     op.OpID,
				ExitCode: 2,
				Stdout:   "",
				Stderr:   "plan operation args are empty; refusing execution to avoid false-positive help output",
			})
			resp.Status = "failed"
			resp.Error = "operation failed: " + op.OpID
			return resp
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

		cmd := exec.Command(alfrescoBinary(), args...)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		exitCode := 0
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
				if stderr.Len() == 0 {
					stderr.WriteString(err.Error())
				}
			}
		}

		result := ApplyResult{
			OpID:     op.OpID,
			ExitCode: exitCode,
			Stdout:   strings.TrimSpace(stdout.String()),
			Stderr:   strings.TrimSpace(stderr.String()),
		}
		if exitCode == 0 && looksLikeTopLevelHelp(result.Stdout) {
			result.ExitCode = 2
			if result.Stderr == "" {
				result.Stderr = "unexpected top-level alfresco help output; refusing to mark operation as succeeded"
			}
		}
		resp.Results = append(resp.Results, result)
		if result.ExitCode != 0 {
			resp.Status = "failed"
			resp.Error = "operation failed: " + op.OpID
			return resp
		}
	}

	resp.Status = "succeeded"
	resp.Artifacts = map[string]any{"plan_id": req.PlanID}
	_ = markApplied(req.IdempotencyKey, resp)
	return resp
}

func fetchAllChildren(nodeID string) ([]nodeEntry, error) {
	all := []nodeEntry{}
	skip := 0
	for {
		args := []string{"node", "list", "-i", nodeID, "--skipCount", strconv.Itoa(skip), "--maxItems", "1000", "--format", "json"}
		out, _, exit, err := runAlfresco(args)
		if err != nil || exit != 0 {
			if err == nil {
				err = fmt.Errorf("alfresco exit code %d", exit)
			}
			return nil, err
		}
		var parsed nodeListResponse
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			return nil, err
		}
		for _, item := range parsed.List.Entries {
			all = append(all, item.Entry)
		}
		if !parsed.List.Pagination.HasMoreItems || parsed.List.Pagination.Count == 0 {
			break
		}
		skip += parsed.List.Pagination.Count
	}
	return all, nil
}

func buildCandidate(req ResolveRequest, siteID string, path string, entry nodeEntry) ResolveCandidate {
	kind := "folder"
	if entry.IsFile {
		kind = "file"
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(entry.Name)), ".")
	breakdown := ScoreBreakdown{}
	reasons := []string{}

	breakdown.Path = scorePath(req, path)
	if breakdown.Path > 0 {
		reasons = append(reasons, "path matches provided scope/hints")
	}

	breakdown.Name = scoreName(req.Target.Name, entry.Name)
	if breakdown.Name >= 24.9 {
		reasons = append(reasons, "exact name match")
	}

	breakdown.TypeMeta = scoreTypeMeta(req.Target.Extension, ext)
	breakdown.Semantic = scoreSemantic(req.Hints.QueryText, path+" "+entry.Name)
	breakdown.Recency = scoreRecency(entry.ModifiedAt, req.Hints.ModifiedWithinDays)
	breakdown.History = scoreHistory(req, entry.ID)

	score := breakdown.Path + breakdown.Name + breakdown.TypeMeta + breakdown.Semantic + breakdown.Recency + breakdown.History
	if score > 100 {
		score = 100
	}

	return ResolveCandidate{
		NodeID:         entry.ID,
		SiteID:         siteID,
		FullPath:       path,
		Kind:           kind,
		Score:          round2(score),
		ScoreBreakdown: breakdown,
		Reasons:        reasons,
		modifiedAt:     entry.ModifiedAt,
		createdBy:      entry.CreatedByUser.ID,
		extension:      ext,
		normalizedName: normalize(entry.Name),
	}
}

func applyDuplicatePenalties(candidates []ResolveCandidate) {
	nameSiteCounts := map[string]map[string]int{}
	for _, c := range candidates {
		name := c.normalizedName
		if _, ok := nameSiteCounts[name]; !ok {
			nameSiteCounts[name] = map[string]int{}
		}
		nameSiteCounts[name][c.SiteID]++
	}
	for i := range candidates {
		sites := nameSiteCounts[candidates[i].normalizedName]
		if len(sites) > 1 {
			candidates[i].ScoreBreakdown.Penalty += 15
			candidates[i].Score = round2(math.Max(0, candidates[i].Score-15))
			candidates[i].Reasons = append(candidates[i].Reasons, "duplicate basename appears across multiple sites")
		}
	}
}

func scorePath(req ResolveRequest, path string) float64 {
	normPath := normalize(path)
	if req.Target.ExpectedParentPath != "" {
		normExpected := normalize(req.Target.ExpectedParentPath)
		if strings.HasSuffix(normPath, normExpected) {
			return 30
		}
		if strings.Contains(normPath, normExpected) {
			return 20
		}
	}
	if len(req.Hints.AncestorNames) == 0 {
		return 0
	}
	matches := 0
	for _, segment := range req.Hints.AncestorNames {
		if strings.Contains(normPath, normalize(segment)) {
			matches++
		}
	}
	return round2(math.Min(22, float64(matches)*7.5))
}

func scoreName(target string, candidate string) float64 {
	t := normalize(target)
	c := normalize(candidate)
	if t == "" || c == "" {
		return 0
	}
	if t == c {
		return 25
	}
	if strings.TrimSuffix(t, filepath.Ext(t)) == strings.TrimSuffix(c, filepath.Ext(c)) {
		return 20
	}
	return round2(diceCoefficient(t, c) * 24)
}

func scoreTypeMeta(expectedExt, ext string) float64 {
	if strings.TrimSpace(expectedExt) == "" {
		return 0
	}
	expected := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(expectedExt)), ".")
	if expected == ext {
		return 15
	}
	if expected != "" && strings.Contains(ext, expected) {
		return 8
	}
	return 0
}

func scoreSemantic(query string, context string) float64 {
	qt := tokenize(query)
	ct := tokenize(context)
	if len(qt) == 0 || len(ct) == 0 {
		return 0
	}
	cset := map[string]bool{}
	for _, tok := range ct {
		cset[tok] = true
	}
	matches := 0
	for _, tok := range qt {
		if cset[tok] {
			matches++
		}
	}
	return round2(math.Min(10, float64(matches)*2.5))
}

func scoreRecency(modifiedAt string, withinDays int) float64 {
	if withinDays <= 0 || strings.TrimSpace(modifiedAt) == "" {
		return 0
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05-0700"}
	var parsed time.Time
	var err error
	for _, layout := range layouts {
		parsed, err = time.Parse(layout, modifiedAt)
		if err == nil {
			break
		}
	}
	if err != nil {
		return 0
	}
	if time.Since(parsed) <= time.Duration(withinDays)*24*time.Hour {
		return 10
	}
	return 0
}

func scoreHistory(req ResolveRequest, nodeID string) float64 {
	path := memoryPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	memo := map[string]string{}
	if err := json.Unmarshal(raw, &memo); err != nil {
		return 0
	}
	key := historyKey(req)
	if memo[key] == nodeID {
		return 10
	}
	return 0
}

func confidenceBand(top, delta float64) string {
	if top >= 85 && delta >= 15 {
		return "high"
	}
	if top >= 70 && delta >= 8 {
		return "medium"
	}
	return "low"
}

func matchesKind(kind string, isFile, isFolder bool) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "file":
		return isFile
	case "folder":
		return isFolder
	default:
		return true
	}
}

func loadResolveRequest(filePath, inline string) (ResolveRequest, error) {
	var req ResolveRequest
	if filePath != "" {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return req, err
		}
		return req, json.Unmarshal(raw, &req)
	}
	if inline != "" {
		return req, json.Unmarshal([]byte(inline), &req)
	}
	return req, errors.New("no resolve request payload provided")
}

func loadPlanRequest(filePath, inline string) (PlanRequest, error) {
	var req PlanRequest
	if filePath != "" {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return req, err
		}
		return req, json.Unmarshal(raw, &req)
	}
	if inline != "" {
		return req, json.Unmarshal([]byte(inline), &req)
	}
	return req, errors.New("no plan request payload provided")
}

func loadApplyRequest(filePath, inline string) (ApplyRequest, error) {
	var req ApplyRequest
	if filePath != "" {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return req, err
		}
		return req, json.Unmarshal(raw, &req)
	}
	if inline != "" {
		return req, json.Unmarshal([]byte(inline), &req)
	}
	return req, errors.New("no apply request payload provided")
}

func savePlan(resp PlanResponse) error {
	planDir := planStateDir()
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return err
	}
	rec := storedPlan{PlanResponse: resp}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(planDir, resp.PlanID+".json"), raw, 0o644)
}

func loadPlan(planID string) (PlanResponse, error) {
	var rec storedPlan
	raw, err := os.ReadFile(filepath.Join(planStateDir(), planID+".json"))
	if err != nil {
		return PlanResponse{}, err
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return PlanResponse{}, err
	}
	return rec.PlanResponse, nil
}

func alreadyApplied(idempotencyKey string) bool {
	_, err := os.Stat(filepath.Join(appliedStateDir(), idempotencyKey+".json"))
	return err == nil
}

func markApplied(idempotencyKey string, resp ApplyResponse) error {
	if err := os.MkdirAll(appliedStateDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(appliedStateDir(), idempotencyKey+".json"), raw, 0o644)
}

func planStateDir() string {
	return filepath.Join(skillRoot(), "state", "plans")
}

func appliedStateDir() string {
	return filepath.Join(skillRoot(), "state", "applied")
}

func memoryPath() string {
	return filepath.Join(skillRoot(), "state", "memory.json")
}

func skillRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "skills/alfresco-agent"
	}
	if strings.Contains(cwd, string(filepath.Separator)+"skills"+string(filepath.Separator)+"alfresco-agent") {
		return cwd
	}
	return filepath.Join(cwd, "skills", "alfresco-agent")
}

func runAlfresco(args []string) (string, string, int, error) {
	args = withCredentials(args)
	cmd := exec.Command(alfrescoBinary(), args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), exitCode, err
}

func withCredentials(args []string) []string {
	username := strings.TrimSpace(os.Getenv("ALFRESCO_USERNAME"))
	password := strings.TrimSpace(os.Getenv("ALFRESCO_PASSWORD"))
	if username == "" || password == "" {
		return args
	}

	hasUsername := false
	hasPassword := false
	for i := 0; i < len(args); i++ {
		if args[i] == "--username" {
			hasUsername = true
		}
		if args[i] == "--password" {
			hasPassword = true
		}
	}
	if hasUsername || hasPassword {
		return args
	}

	withAuth := make([]string, 0, len(args)+4)
	withAuth = append(withAuth, args...)
	withAuth = append(withAuth, "--username", username, "--password", password)
	return withAuth
}

func alfrescoBinary() string {
	if custom := strings.TrimSpace(os.Getenv("ALFRESCO_BIN")); custom != "" {
		return custom
	}
	return "alfresco"
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer("_", " ", "-", " ", "/", " ")
	s = replacer.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func tokenize(s string) []string {
	norm := normalize(s)
	if norm == "" {
		return nil
	}
	return strings.Fields(norm)
}

func diceCoefficient(a, b string) float64 {
	if len(a) < 2 || len(b) < 2 {
		if a == b {
			return 1
		}
		return 0
	}
	aBigrams := map[string]int{}
	for i := 0; i < len(a)-1; i++ {
		aBigrams[a[i:i+2]]++
	}
	matches := 0
	for i := 0; i < len(b)-1; i++ {
		bg := b[i : i+2]
		if aBigrams[bg] > 0 {
			matches++
			aBigrams[bg]--
		}
	}
	return (2.0 * float64(matches)) / float64((len(a)-1)+(len(b)-1))
}

func shellJoin(parts []string) string {
	escaped := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			escaped = append(escaped, "''")
			continue
		}
		if strings.IndexFunc(p, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '"' || r == '\'' || r == '$' || r == '&' || r == '|' || r == ';' || r == '<' || r == '>'
		}) >= 0 {
			escaped = append(escaped, "'"+strings.ReplaceAll(p, "'", "'\\''")+"'")
		} else {
			escaped = append(escaped, p)
		}
	}
	return strings.Join(escaped, " ")
}

func looksLikeTopLevelHelp(stdout string) bool {
	out := strings.TrimSpace(stdout)
	if out == "" {
		return false
	}
	return strings.Contains(out, "Alfresco CLI provides access to Alfresco REST API services via command line.") &&
		strings.Contains(out, "Usage:\n  alfresco [command]") &&
		strings.Contains(out, "Available Commands:")
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func fallbackID(preferred string, prefix string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		return preferred
	}
	return newID(prefix)
}

func mergeBool(current bool, defaultValue bool, raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		if !current {
			return defaultValue
		}
		return current
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d-%04d", prefix, time.Now().Unix(), rand.Intn(10000))
}

func historyKey(req ResolveRequest) string {
	scope := ""
	if len(req.Scope.AllowedRoots) > 0 {
		scope = req.Scope.AllowedRoots[0].SiteID + ":" + req.Scope.AllowedRoots[0].RootNodeID
	}
	return strings.Join([]string{
		normalize(req.Operation),
		normalize(req.Target.Kind),
		normalize(req.Target.Name),
		normalize(req.Target.ExpectedParentPath),
		normalize(scope),
	}, "|")
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		printErrorAndExit(err.Error())
	}
}

func printErrorAndExit(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

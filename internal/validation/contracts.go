package validation

import "time"

const SchemaVersion = "1.0"

type ErrorDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	TraceID string        `json:"trace_id"`
	Details []ErrorDetail `json:"details,omitempty"`
}

type ResolveRequest struct {
	SchemaVersion string `json:"schema_version"`
	Operation     string `json:"operation"`
	Scope         struct {
		SiteID     string `json:"site_id"`
		RootNodeID string `json:"root_node_id"`
		MaxDepth   int    `json:"max_depth"`
	} `json:"scope"`
	Target struct {
		Kind               string   `json:"kind"`
		Name               string   `json:"name"`
		Extension          string   `json:"extension,omitempty"`
		ExpectedParentPath string   `json:"expected_parent_path,omitempty"`
		ExpectedSiteID     string   `json:"expected_site_id,omitempty"`
		AncestorNames      []string `json:"ancestor_names,omitempty"`
		QueryText          string   `json:"query_text,omitempty"`
		ModifiedWithinDays int      `json:"modified_within_days,omitempty"`
	} `json:"target"`
	Policy struct {
		MaxCandidates int  `json:"max_candidates"`
		RequireUnique bool `json:"require_unique"`
	} `json:"policy"`
}

type ResolveCandidate struct {
	NodeID   string  `json:"node_id"`
	SiteID   string  `json:"site_id"`
	FullPath string  `json:"full_path"`
	Kind     string  `json:"kind"`
	Score    float64 `json:"score"`
}

type Confidence struct {
	TopScore    float64 `json:"top_score"`
	SecondScore float64 `json:"second_score"`
	Delta       float64 `json:"delta"`
	Band        string  `json:"band"`
}

type ResolveResponse struct {
	SchemaVersion       string             `json:"schema_version"`
	RequestID           string             `json:"request_id"`
	Status              string             `json:"status"`
	BestCandidateNodeID string             `json:"best_candidate_node_id,omitempty"`
	Candidates          []ResolveCandidate `json:"candidates"`
	Confidence          Confidence         `json:"confidence"`
	NextAction          string             `json:"next_action"`
	Questions           []string           `json:"questions,omitempty"`
}

type PlanRequest struct {
	SchemaVersion    string `json:"schema_version"`
	ResolveRequestID string `json:"resolve_request_id"`
	Action           string `json:"action"`
	Selection        struct {
		TargetNodeID       string `json:"target_node_id,omitempty"`
		TargetParentNodeID string `json:"target_parent_node_id,omitempty"`
	} `json:"selection"`
	Payload struct {
		LocalFilePath string            `json:"local_file_path,omitempty"`
		NewName       string            `json:"new_name,omitempty"`
		Properties    map[string]string `json:"properties,omitempty"`
	} `json:"payload"`
	Safety struct {
		DryRun               bool   `json:"dry_run"`
		RequirePreconditions bool   `json:"require_preconditions"`
		ExpectedModifiedAt   string `json:"expected_modified_at,omitempty"`
		ExpectedChecksum     string `json:"expected_checksum,omitempty"`
	} `json:"safety"`
}

type PlanOperation struct {
	OpID        string `json:"op_id"`
	Description string `json:"description"`
	CLICommand  string `json:"cli_command"`
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
}

type ApplyRequest struct {
	SchemaVersion  string `json:"schema_version"`
	PlanID         string `json:"plan_id"`
	PlanHash       string `json:"plan_hash"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	Approved       bool   `json:"approved,omitempty"`
	Runtime        struct {
		HTTPTimeout   string `json:"http_timeout,omitempty"`
		HTTPRetries   int    `json:"http_retries,omitempty"`
		HTTPRetryWait string `json:"http_retry_wait,omitempty"`
	} `json:"runtime"`
}

type ApplyResult struct {
	OpID     string `json:"op_id"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type ApplyResponse struct {
	SchemaVersion string        `json:"schema_version"`
	RequestID     string        `json:"request_id"`
	ExecutionID   string        `json:"execution_id"`
	Status        string        `json:"status"`
	ApprovalID    string        `json:"approval_id,omitempty"`
	Results       []ApplyResult `json:"results,omitempty"`
	Error         string        `json:"error,omitempty"`
}

type ApprovalDecisionRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

type ApprovalDecisionResponse struct {
	ApprovalID string    `json:"approval_id"`
	Status     string    `json:"status"`
	DecidedBy  string    `json:"decided_by"`
	DecidedAt  time.Time `json:"decided_at"`
}

type OperationStatusResponse struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	TraceID     string `json:"trace_id"`
	Result      any    `json:"result,omitempty"`
}

type AuditEvent struct {
	EventID    string         `json:"event_id"`
	TraceID    string         `json:"trace_id"`
	EventType  string         `json:"event_type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Actor      string         `json:"actor"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type AuditEventsResponse struct {
	TraceID string       `json:"trace_id"`
	Events  []AuditEvent `json:"events"`
}

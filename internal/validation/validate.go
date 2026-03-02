package validation

import (
	"fmt"
	"regexp"
	"strings"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

func ValidateResolveRequest(r ResolveRequest) []ErrorDetail {
	var errs []ErrorDetail
	if r.SchemaVersion != SchemaVersion {
		errs = append(errs, ErrorDetail{Field: "schema_version", Message: "must be 1.0"})
	}
	switch r.Operation {
	case "locate_for_update", "locate_for_upload", "locate_for_create":
	default:
		errs = append(errs, ErrorDetail{Field: "operation", Message: "invalid operation"})
	}
	if r.Scope.SiteID == "" {
		errs = append(errs, ErrorDetail{Field: "scope.site_id", Message: "is required"})
	}
	if !isUUID(r.Scope.RootNodeID) {
		errs = append(errs, ErrorDetail{Field: "scope.root_node_id", Message: "must be UUID"})
	}
	if r.Scope.MaxDepth < 1 || r.Scope.MaxDepth > 10 {
		errs = append(errs, ErrorDetail{Field: "scope.max_depth", Message: "must be between 1 and 10"})
	}
	if r.Target.Kind != "file" && r.Target.Kind != "folder" {
		errs = append(errs, ErrorDetail{Field: "target.kind", Message: "must be file or folder"})
	}
	if strings.TrimSpace(r.Target.Name) == "" {
		errs = append(errs, ErrorDetail{Field: "target.name", Message: "is required"})
	}
	if r.Policy.MaxCandidates < 1 || r.Policy.MaxCandidates > 20 {
		errs = append(errs, ErrorDetail{Field: "policy.max_candidates", Message: "must be between 1 and 20"})
	}
	return errs
}

func ValidatePlanRequest(r PlanRequest) []ErrorDetail {
	var errs []ErrorDetail
	if r.SchemaVersion != SchemaVersion {
		errs = append(errs, ErrorDetail{Field: "schema_version", Message: "must be 1.0"})
	}
	switch r.Action {
	case "upload_new_version", "update_metadata", "create_child":
	default:
		errs = append(errs, ErrorDetail{Field: "action", Message: "invalid action"})
	}
	if strings.TrimSpace(r.ResolveRequestID) == "" {
		errs = append(errs, ErrorDetail{Field: "resolve_request_id", Message: "is required"})
	}
	if r.Action == "create_child" {
		if !isUUID(r.Selection.TargetParentNodeID) {
			errs = append(errs, ErrorDetail{Field: "selection.target_parent_node_id", Message: "must be UUID for create_child"})
		}
	} else if !isUUID(r.Selection.TargetNodeID) {
		errs = append(errs, ErrorDetail{Field: "selection.target_node_id", Message: "must be UUID"})
	}
	return errs
}

func ValidateApplyRequest(r ApplyRequest) []ErrorDetail {
	var errs []ErrorDetail
	if r.SchemaVersion != SchemaVersion {
		errs = append(errs, ErrorDetail{Field: "schema_version", Message: "must be 1.0"})
	}
	if strings.TrimSpace(r.PlanID) == "" {
		errs = append(errs, ErrorDetail{Field: "plan_id", Message: "is required"})
	}
	if strings.TrimSpace(r.PlanHash) == "" {
		errs = append(errs, ErrorDetail{Field: "plan_hash", Message: "is required"})
	}
	return errs
}

func ValidateResolveResponseDeterministic(resp ResolveResponse) error {
	if resp.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version mismatch")
	}
	switch resp.Status {
	case "resolved", "ambiguous", "not_found", "out_of_scope":
	default:
		return fmt.Errorf("invalid status: %s", resp.Status)
	}
	for i, c := range resp.Candidates {
		if !isUUID(c.NodeID) {
			return fmt.Errorf("candidates[%d].node_id must be UUID", i)
		}
	}
	return nil
}

func isUUID(v string) bool {
	return uuidRe.MatchString(v)
}

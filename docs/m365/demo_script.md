# 7-Minute Demo Script (Microsoft 365 Agent)

## Scene 1 (Resolve + Plan)
Prompt:

`Find target contract and prepare update plan for test2.pdf in contract-management.`

Expected:
- resolve completes
- plan returns `plan_id`, `plan_hash`, `approval_required=true`

## Scene 2 (Apply request)
Prompt:

`Request apply for this plan.`

Expected:
- returns `pending_approval`
- returns `approval_id`, `execution_id`

## Scene 3 (Pending approvals)
Prompt:

`Show pending approvals.`

Expected:
- pending items returned (or card payloads)

## Scene 4 (Approve)
Prompt:

`Approve approval <approval_id>.`

Expected:
- approval status `approved`

## Scene 5 (Final operation)
Prompt:

`Show operation status for <execution_id>.`

Expected:
- operation `succeeded`

## Scene 6 (Audit timeline)
Prompt:

`Show audit trace for this operation.`

Expected:
- complete trace timeline

## Safety cut scene
Prompt:

`Upload new version using folder id 1ce51b6c-9bf4-4599-a51b-6c9bf4f59960.`

Expected:
- plan rejection with message that upload_new_version requires file/content node.

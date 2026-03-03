#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-https://alfagent.crestsolution.com:7443}"
TOKEN="${TOKEN:-}"

if [[ -z "$TOKEN" ]]; then
  echo "TOKEN is required (export TOKEN=...)" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

ID_SUFFIX="$(date +%s)"

# Inputs for demo
ROOT_NODE_ID="${ROOT_NODE_ID:-1ce51b6c-9bf4-4599-a51b-6c9bf4f59960}"
PARENT_FOLDER_ID="${PARENT_FOLDER_ID:-1ce51b6c-9bf4-4599-a51b-6c9bf4f59960}"
LOCAL_FILE_PATH="${LOCAL_FILE_PATH:-/data/venvs/vlmmpdf/test2.pdf}"
NEW_NAME="${NEW_NAME:-test2_demo_${ID_SUFFIX}.pdf}"
SITE_ID="${SITE_ID:-contract-management}"

echo "[1/6] Resolve"
RESOLVE_JSON=$(curl -s -X POST "$BASE_URL/v1/resolve" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Trace-Id: demo-$ID_SUFFIX" \
  -d "{\"schema_version\":\"1.0\",\"operation\":\"locate_for_create\",\"scope\":{\"site_id\":\"$SITE_ID\",\"root_node_id\":\"$ROOT_NODE_ID\",\"max_depth\":6},\"target\":{\"kind\":\"folder\",\"name\":\"contract-management\",\"expected_site_id\":\"$SITE_ID\"},\"policy\":{\"max_candidates\":5,\"require_unique\":false}}")
echo "$RESOLVE_JSON" | jq


echo "[2/6] Plan (create_child upload)"
PLAN_JSON=$(curl -s -X POST "$BASE_URL/v1/plan" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: demo-plan-$ID_SUFFIX" \
  -d "{\"schema_version\":\"1.0\",\"resolve_request_id\":\"demo-r-$ID_SUFFIX\",\"action\":\"create_child\",\"selection\":{\"target_parent_node_id\":\"$PARENT_FOLDER_ID\"},\"payload\":{\"new_name\":\"$NEW_NAME\",\"local_file_path\":\"$LOCAL_FILE_PATH\"},\"safety\":{\"dry_run\":true,\"require_preconditions\":false}}")
echo "$PLAN_JSON" | jq
PLAN_ID=$(echo "$PLAN_JSON" | jq -r '.plan_id')
PLAN_HASH=$(echo "$PLAN_JSON" | jq -r '.plan_hash')


echo "[3/6] Apply request (pending approval)"
APPLY_JSON=$(curl -s -X POST "$BASE_URL/v1/apply" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: demo-apply-$ID_SUFFIX" \
  -d "{\"schema_version\":\"1.0\",\"plan_id\":\"$PLAN_ID\",\"plan_hash\":\"$PLAN_HASH\"}")
echo "$APPLY_JSON" | jq
APPROVAL_ID=$(echo "$APPLY_JSON" | jq -r '.approval_id')
EXECUTION_ID=$(echo "$APPLY_JSON" | jq -r '.execution_id')


echo "[4/6] Pending approval cards"
curl -s "$BASE_URL/v1/approvals/pending/cards?limit=10" \
  -H "Authorization: Bearer $TOKEN" | jq


echo "[5/6] Approve"
curl -s -X POST "$BASE_URL/v1/approvals/$APPROVAL_ID/decision" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"decision":"approve","reason":"approved by demo script"}' | jq


echo "[6/6] Operation status"
OP_JSON=$(curl -s "$BASE_URL/v1/operations/$EXECUTION_ID" \
  -H "Authorization: Bearer $TOKEN")
echo "$OP_JSON" | jq

echo "Done. execution_id=$EXECUTION_ID approval_id=$APPROVAL_ID"

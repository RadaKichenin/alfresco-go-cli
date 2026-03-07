# Copilot Agent Import Guide

## Package contents

- `openapi.import.yaml` : import this into Copilot/M365 Agent actions.
- `agent.package.json` : metadata and behavior rules for this package.
- `../agent_instructions.md` : paste into agent instructions.
- `../actions_definitions.json` : optional JSON reference for action metadata.

## Import steps (Microsoft 365 Agent)

1. Go to Agent UI -> create/edit your agent.
2. Add/Import actions from OpenAPI:
   - Use `openapi.import.yaml`.
3. Configure auth connection:
   - Bearer token via Entra service account.
   - audience: `715bd2b9-a0e9-4080-8f48-fdff0ecd3253`.
4. Paste system behavior from `agent_instructions.md`.
5. Save and publish to test scope.

## Validate actions quickly

- `resolve_target`
- `create_plan`
- `request_apply`
- `list_pending_approval_cards`
- `decide_approval`
- `get_operation_status`

## Build distributable zip

Run from repo root:

```bash
cd docs/m365
zip -r copilot-agent-import.zip copilot-package agent_instructions.md actions_definitions.json demo_script.md service_account_auth.md
```

This zip is the handoff package for demo setup.

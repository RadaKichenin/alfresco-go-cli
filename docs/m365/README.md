# Microsoft 365 Agent Demo Integration (Alfresco Gateway)

This folder contains copy-paste assets to build a quick Microsoft 365 Agent demo against:

- `https://alfagent.crestsolution.com:7443`
- Entra auth mode on gateway (`-auth-mode=entra`)
- Approval workflow (`resolve -> plan -> apply(pending) -> approve -> operation status`)

## Files

- `agent_instructions.md` : system instructions for the M365 agent.
- `actions_definitions.json` : action metadata + input schemas.
- `demo_script.md` : 7-minute talk track and exact prompts.
- `service_account_auth.md` : Entra service account setup checklist.
- `../../scripts/demo/m365_demo_flow.sh` : curl fallback for full E2E.

## Demo prerequisites

1. Gateway is reachable via reverse proxy URL.
2. Entra auth configured:
   - tenant: `89a93711-19eb-4ef4-8131-439d503d08c7`
   - audience: `715bd2b9-a0e9-4080-8f48-fdff0ecd3253`
3. Service account has app roles on API app:
   - `Operator`
   - `Approver`
   - (optional) `Reader`
4. `token-debug` disabled in production demos.

## M365 Agent quick setup

1. Create new agent in Microsoft 365 Agent UI.
2. Paste instructions from `agent_instructions.md`.
3. Add actions from `actions_definitions.json` mapped to your API base URL.
4. Configure connection using Entra service account.
5. Run prompts from `demo_script.md`.

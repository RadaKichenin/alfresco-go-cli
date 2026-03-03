# Entra Service Account Setup (Demo)

## Goal
Use one service account connection in M365 Agent actions to call Alfresco gateway.

## Required API app settings

- Tenant: `89a93711-19eb-4ef4-8131-439d503d08c7`
- Audience: `715bd2b9-a0e9-4080-8f48-fdff0ecd3253`
- Access token version: `2`

## App role assignment

Assign service account (or its security group) on API Enterprise Application with roles:

- `Operator`
- `Approver`
- Optional: `Reader`

## Verification

1. Acquire token for audience.
2. Call:

```bash
curl -s https://alfagent.crestsolution.com:7443/dev/token-debug \
  -H "Authorization: Bearer $TOKEN" | jq
```

3. Confirm roles include assigned values.

## Production note

Disable debug endpoint in production:

- gateway flag: `-token-debug=false`

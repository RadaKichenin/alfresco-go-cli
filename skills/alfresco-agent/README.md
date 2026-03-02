# alfresco-agent skill

Go-based wrappers implementing a safe `resolve -> plan -> apply` workflow for the Alfresco CLI.

## Commands

```bash
go run ./skills/alfresco-agent/cmd/alfresco-agent/main.go resolve ...
go run ./skills/alfresco-agent/cmd/alfresco-agent/main.go plan ...
go run ./skills/alfresco-agent/cmd/alfresco-agent/main.go apply ...
```

## Environment

- `ALFRESCO_BIN`: optional path to the alfresco executable (default: `alfresco`)

## State files

- Plans: `skills/alfresco-agent/state/plans/*.json`
- Idempotency execution markers: `skills/alfresco-agent/state/applied/*.json`

## Resolve example

```bash
go run ./skills/alfresco-agent/cmd/alfresco-agent/main.go resolve \
  --operation locate_for_update \
  --kind file \
  --name Q1-report.xlsx \
  --site-id legal \
  --root-node-id SOME_ROOT_NODE_ID \
  --expected-parent-path /legal/Finance/Reports \
  --max-depth 5 \
  --max-candidates 5 \
  --require-unique true
```

## Plan example

```bash
go run ./skills/alfresco-agent/cmd/alfresco-agent/main.go plan \
  --resolve-request-id resolve-123 \
  --action upload_new_version \
  --target-node-id NODE_ID \
  --local-file-path /tmp/Q1-report.xlsx \
  --dry-run true \
  --require-preconditions true
```

## Apply example

```bash
go run ./skills/alfresco-agent/cmd/alfresco-agent/main.go apply \
  --plan-id plan-123 \
  --plan-hash PLAN_HASH \
  --idempotency-key upload-q1-report-2026-03-02 \
  --approved true
```

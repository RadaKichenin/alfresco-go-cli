# Alfresco CLI (Agent-Friendly Guide)

This is a practical guide for using `alfresco` as both:
- a human-friendly command line tool
- a machine-friendly executable inside AI agent frameworks (OpenClaw-style tools, LangChain, AutoGen, CrewAI, custom orchestrators)

## 1. What This CLI Does

`alfresco` talks to Alfresco Content Services (ACS) via REST APIs.

Main command groups:
- `config`: connection and credentials setup
- `node`: file/folder operations
- `people`: user operations
- `group`: group operations

## 2. Install and Build

From repo root:

```bash
go build -o alfresco ./alfresco.go
```

Or use make targets:

```bash
make build
```

## 3. First-Time Setup

Store ACS connection and credentials:

```bash
./alfresco config set -s http://localhost:8080/alfresco -u admin -p admin
```

Override credentials per command (recommended for agent runs):

```bash
./alfresco node list -i -root- --username admin --password admin
```

## 4. Output Modes (Important for Agents)

Preferred flag:
- `--format` (alias: `--output`)

Supported values:
- `table` (human-readable)
- `json` (agent-readable)
- `id` (line-separated IDs)

Examples:

```bash
./alfresco node list -i -root- --format json
./alfresco node list -i -root- --format table
./alfresco node list -i -root- --format id
```

### Non-interactive default behavior
If stdout is not a TTY (typical agent subprocess), default output automatically becomes JSON.

This means an agent can usually run commands without forcing `--format json`, but setting it explicitly is still best practice.

## 5. Stream and Exit-Code Contract

For automation safety:
- Success payloads are written to `stdout`
- Failures are written to `stderr`
- Non-zero exit code indicates failure

Agent logic should always:
1. check process exit code
2. parse `stdout` only on success
3. capture `stderr` for diagnosis on failures

## 6. HTTP Reliability Controls

Global flags for robust agent execution:

- `--http-timeout` (default `30s`)
- `--http-retries` (default `2`)
- `--http-retry-wait` (default `500ms`)

Example:

```bash
./alfresco node get -i SOME_NODE_ID \
  --username admin --password admin \
  --http-timeout 20s \
  --http-retries 3 \
  --http-retry-wait 1s \
  --format json
```

Retry-safe behavior is applied for methods like `GET`, `PUT`, and `DELETE`.

## 7. Common Agent-Friendly Commands

List children of root as JSON:

```bash
./alfresco node list -i -root- --format json --username admin --password admin
```

Get a node:

```bash
./alfresco node get -i NODE_ID --format json --username admin --password admin
```

Create folder:

```bash
./alfresco node create -i -root- -n my-folder -t cm:folder --format json --username admin --password admin
```

Upload a file as a content node:

```bash
./alfresco node create -i -root- -n report.pdf -t cm:content -f ./report.pdf --format json --username admin --password admin
```

Delete node:

```bash
./alfresco node delete -i NODE_ID --username admin --password admin
```

## 8. OpenClaw / Agent Framework Integration Pattern

If your framework supports a command tool, define a tool that executes `alfresco` as a subprocess.

### Required runtime behavior
- Provide command args as a list (avoid shell string concatenation when possible)
- Capture `stdout`, `stderr`, and `exit_code`
- Parse JSON only when `exit_code == 0`

### Generic tool wrapper contract

Input schema idea:

```json
{
  "args": ["node", "list", "-i", "-root-", "--format", "json"],
  "username": "admin",
  "password": "admin"
}
```

Execution behavior:
- append `--username` and `--password` when provided
- run binary
- return:
  - `exit_code`
  - `stdout`
  - `stderr`
  - `parsed_json` (optional, only on success and valid JSON)

## 9. Python Integration Example (Subprocess)

```python
import json
import subprocess


def run_alfresco(args, username=None, password=None):
    cmd = ["./alfresco", *args]
    if username and password:
        cmd += ["--username", username, "--password", password]

    p = subprocess.run(cmd, capture_output=True, text=True)

    result = {
        "exit_code": p.returncode,
        "stdout": p.stdout,
        "stderr": p.stderr,
    }

    if p.returncode == 0:
        try:
            result["json"] = json.loads(p.stdout)
        except json.JSONDecodeError:
            result["json"] = None

    return result


res = run_alfresco(
    ["node", "list", "-i", "-root-", "--format", "json"],
    username="admin",
    password="admin",
)

if res["exit_code"] != 0:
    raise RuntimeError(res["stderr"])

print(res["json"])
```

## 10. Node.js Integration Example (Child Process)

```javascript
import { spawn } from "node:child_process";

function runAlfresco(args, username, password) {
  return new Promise((resolve) => {
    const fullArgs = [...args];
    if (username && password) {
      fullArgs.push("--username", username, "--password", password);
    }

    const child = spawn("./alfresco", fullArgs, { stdio: ["ignore", "pipe", "pipe"] });

    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (d) => (stdout += d.toString()));
    child.stderr.on("data", (d) => (stderr += d.toString()));

    child.on("close", (code) => {
      let parsed = null;
      if (code === 0) {
        try {
          parsed = JSON.parse(stdout);
        } catch {
          parsed = null;
        }
      }
      resolve({ code, stdout, stderr, parsed });
    });
  });
}
```

## 11. MCP / Tool-Calling Best Practices

When exposing this CLI as a tool in MCP-like environments:
- Force `--format json` in the tool wrapper
- Return structured object fields (`exit_code`, `stderr`, `data`)
- Redact secrets from logs
- Use conservative retries in the wrapper only if command-level retries are disabled

Recommended wrapper defaults:
- `--http-timeout 30s`
- `--http-retries 2`
- `--http-retry-wait 500ms`

## 12. CI and Contract Gates

Available make targets:

```bash
make test-unit      # package tests
make test-contract  # binary-level CLI contract tests
make ci-test        # unit + contract tests
```

Existing shell-based E2E scripts remain available via:

```bash
make test
```

## 13. Troubleshooting

If command fails:
1. Check `stderr` first
2. Check exit code
3. Re-run with explicit `--format json`
4. Confirm ACS URL and credentials
5. Increase timeout/retries for slow environments

Example:

```bash
./alfresco node list -i -root- \
  --format json \
  --http-timeout 60s \
  --http-retries 4 \
  --http-retry-wait 1s \
  --username admin --password admin
```

## 14. Security Notes

- Avoid hardcoding credentials in agent prompts or source code.
- Prefer secret stores or runtime injection from your orchestrator.
- Avoid logging full command lines if they contain `--password`.

---

If you are integrating with a specific framework (OpenClaw, LangChain, AutoGen, CrewAI, custom MCP server), you can reuse the same subprocess contract from sections 8-11 with minimal changes.

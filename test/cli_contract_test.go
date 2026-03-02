package test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildCLI(t *testing.T) string {
	t.Helper()

	binaryName := "alfresco-test"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)

	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = ".."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build CLI: %v\n%s", err, string(output))
	}

	return binaryPath
}

func writeConfigFile(t *testing.T, dir string, serverURL string) {
	t.Helper()
	config := fmt.Sprintf(`alfresco:
  url: %s
  protocol: http
  insecure: false
  maxItems: 100
`, serverURL)
	if err := os.WriteFile(filepath.Join(dir, ".alfresco"), []byte(config), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
}

func runCLI(t *testing.T, binaryPath string, workdir string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = workdir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("command execution failed unexpectedly: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}

	return exitCode, stdout.String(), stderr.String()
}

func TestCLIAutoJSONOutputInNonTTY(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/-default-/public/alfresco/versions/1/nodes/-root-/children" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"list":{"pagination":{"count":1,"hasMoreItems":false,"totalItems":1,"skipCount":0,"maxItems":100},"entries":[{"entry":{"id":"abc123","name":"Folder","modifiedAt":"2026-01-01T00:00:00.000+0000","modifiedByUser":{"id":"admin","displayName":"Admin"}}}]}}`))
	}))
	defer server.Close()

	binaryPath := buildCLI(t)
	workdir := t.TempDir()
	writeConfigFile(t, workdir, server.URL)

	exitCode, stdout, stderr := runCLI(t,
		binaryPath,
		workdir,
		"node", "list", "-i", "-root-", "--username", "admin", "--password", "admin",
	)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if strings.Contains(stdout, "ID\tNAME") || strings.Contains(stdout, "ID                                   NAME") {
		t.Fatalf("expected json output in non-tty mode, got table output: %q", stdout)
	}
	if !strings.Contains(stdout, `"pagination"`) || !strings.Contains(stdout, `"entries"`) {
		t.Fatalf("expected JSON payload in stdout, got: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on success, got: %q", stderr)
	}
}

func TestCLIHTTPErrorInStderrWithNonZeroExit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/-default-/public/alfresco/versions/1/nodes/bad-node" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"briefSummary":"boom"}}`))
	}))
	defer server.Close()

	binaryPath := buildCLI(t)
	workdir := t.TempDir()
	writeConfigFile(t, workdir, server.URL)

	exitCode, stdout, stderr := runCLI(t,
		binaryPath,
		workdir,
		"node", "get", "-i", "bad-node", "--username", "admin", "--password", "admin",
	)

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for 500 response; stdout=%q stderr=%q", stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout on error, got: %q", stdout)
	}
	if !strings.Contains(stderr, "returned 500") {
		t.Fatalf("expected HTTP status details on stderr, got: %q", stderr)
	}
	if !strings.Contains(stderr, "boom") {
		t.Fatalf("expected response body snippet on stderr, got: %q", stderr)
	}
}

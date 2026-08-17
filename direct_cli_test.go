package modacli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDirectCLIEntryPointsAreNotImplemented(t *testing.T) {
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()

	sessionFile := filepath.Join(t.TempDir(), "session.json")
	tests := []struct {
		name      string
		arguments []string
		stdin     string
	}{
		{name: "mcp", arguments: []string{"mcp", "--mode", "direct", "--session-file", sessionFile}},
		{name: "tools", arguments: []string{"tools", "--mode", "direct", "--session-file", sessionFile}},
		{name: "call", arguments: []string{"call", "moda_store_list", "--mode", "direct", "--session-file", sessionFile}},
		{
			name: "auth login",
			arguments: []string{
				"auth", "login", "--api-base-url", server.URL, "--account", "account-1",
				"--password-stdin", "--session-file", sessionFile,
			},
			stdin: "password-1\n",
		},
		{name: "auth orgs", arguments: []string{"auth", "orgs", "--session-file", sessionFile}},
		{name: "auth use-org", arguments: []string{"auth", "use-org", "3", "--session-file", sessionFile}},
		{name: "auth status", arguments: []string{"auth", "status", "--session-file", sessionFile}},
		{name: "auth logout", arguments: []string{"auth", "logout", "--session-file", sessionFile}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("go", append([]string{"run", "./cmd/moda-cli"}, test.arguments...)...)
			command.Stdin = bytes.NewBufferString(test.stdin)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("direct command unexpectedly succeeded: %s", output)
			}
			firstLine := strings.SplitN(string(output), "\n", 2)[0]
			var failure map[string]any
			if decodeErr := json.Unmarshal([]byte(firstLine), &failure); decodeErr != nil {
				t.Fatalf("failure is not JSON: %v\n%s", decodeErr, output)
			}
			if failure["error"] != "not_implemented" || !strings.Contains(failure["message"].(string), "not implemented") {
				t.Fatalf("failure = %#v", failure)
			}
		})
	}

	if requestCount.Load() != 0 {
		t.Fatalf("direct entry points made %d network requests", requestCount.Load())
	}
	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Fatalf("direct entry points created or changed a session file: %v", err)
	}
}

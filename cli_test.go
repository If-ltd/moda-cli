package modacli_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestHelpPublishesModaCLIIdentity(t *testing.T) {
	command := exec.Command("go", "run", "./cmd/moda-cli", "--help")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("moda-cli --help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "moda-cli") {
		t.Fatalf("help output does not identify moda-cli:\n%s", output)
	}
}

func TestCommandFailureIsStructuredJSON(t *testing.T) {
	command := exec.Command("go", "run", "./cmd/moda-cli", "tools", "--mode", "direct", "--session-file", "/missing/moda-session.json")
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("command unexpectedly succeeded: %s", output)
	}
	firstLine := strings.SplitN(string(output), "\n", 2)[0]
	var failure map[string]any
	if decodeErr := json.Unmarshal([]byte(firstLine), &failure); decodeErr != nil {
		t.Fatalf("failure is not JSON: %v\n%s", decodeErr, output)
	}
	if failure["error"] == "" || failure["message"] == "" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestHelpMarksDirectCommandsNotImplemented(t *testing.T) {
	command := exec.Command("go", "run", "./cmd/moda-cli", "--help")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("moda-cli --help failed: %v\n%s", err, output)
	}
	for _, expected := range []string{
		"Available Commands:", "Manage direct-mode authentication (not implemented)",
		"Call one Moda MCP tool", "List Moda MCP tools",
	} {
		if !strings.Contains(string(output), expected) {
			t.Fatalf("help output does not contain %q:\n%s", expected, output)
		}
	}

	authHelp := exec.Command("go", "run", "./cmd/moda-cli", "auth", "--help")
	authOutput, err := authHelp.CombinedOutput()
	if err != nil {
		t.Fatalf("moda-cli auth --help failed: %v\n%s", err, authOutput)
	}
	for _, expected := range []string{"login", "orgs", "use-org", "status", "logout", "not implemented"} {
		if !strings.Contains(string(authOutput), expected) {
			t.Fatalf("auth help output does not contain %q:\n%s", expected, authOutput)
		}
	}
}

func TestHelpCommandDoesNotRequireAClientConnection(t *testing.T) {
	command := exec.Command("go", "run", "./cmd/moda-cli", "help")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("moda-cli help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Usage:") {
		t.Fatalf("help output = %s", output)
	}
}

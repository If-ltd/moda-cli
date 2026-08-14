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

func TestHelpPublishesClientAndDirectCommands(t *testing.T) {
	command := exec.Command("go", "run", "./cmd/moda-cli", "--help")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("moda-cli --help failed: %v\n%s", err, output)
	}
	for _, expected := range []string{
		"moda-cli tools", "moda-cli call", "moda-cli auth login", "moda-cli auth use-org",
	} {
		if !strings.Contains(string(output), expected) {
			t.Fatalf("help output does not contain %q:\n%s", expected, output)
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

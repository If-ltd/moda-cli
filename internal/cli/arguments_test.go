package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseClientMCPConnectionFileOverride(t *testing.T) {
	command, err := Parse([]string{"mcp", "--mode", "client", "--connection-file", "/tmp/moda.json"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if command.Name != "mcp" || command.Mode != "client" || command.ConnectionFile != "/tmp/moda.json" {
		t.Fatalf("Parse() = %+v", command)
	}
}

func TestParseDefaultsToolListingToClientMode(t *testing.T) {
	command, err := Parse([]string{"tools"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if command.Name != "tools" || command.Mode != "client" {
		t.Fatalf("Parse() = %+v", command)
	}
}

func TestParseToolCallInput(t *testing.T) {
	command, err := Parse([]string{"call", "moda_product_query", "--input", `{"page":2}`})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if command.Name != "call" || command.ToolName != "moda_product_query" {
		t.Fatalf("Parse() = %+v", command)
	}
	want := map[string]any{"page": float64(2)}
	if !reflect.DeepEqual(command.Arguments, want) {
		t.Fatalf("arguments = %#v, want %#v", command.Arguments, want)
	}
}

func TestParseToolCallInputFile(t *testing.T) {
	inputFile := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(inputFile, []byte(`{"templateId":"template-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	command, err := Parse([]string{"call", "moda_product_query", "--input-file", inputFile})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := map[string]any{"templateId": "template-1"}
	if !reflect.DeepEqual(command.Arguments, want) {
		t.Fatalf("arguments = %#v, want %#v", command.Arguments, want)
	}
}

func TestParseDirectLoginRequiresPasswordStdin(t *testing.T) {
	command, err := Parse([]string{
		"auth", "login",
		"--api-base-url", "https://api.moda.test",
		"--account", "account-1",
		"--password-stdin",
		"--session-file", "/tmp/session.json",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if command.Name != "auth-login" || command.APIBaseURL != "https://api.moda.test" ||
		command.Account != "account-1" || !command.PasswordStdin || command.SessionFile != "/tmp/session.json" {
		t.Fatalf("Parse() = %+v", command)
	}
}

func TestParseDirectOrganizationCommands(t *testing.T) {
	tests := []struct {
		arguments []string
		name      string
		orgID     string
	}{
		{arguments: []string{"auth", "orgs"}, name: "auth-orgs"},
		{arguments: []string{"auth", "use-org", "3"}, name: "auth-use-org", orgID: "3"},
		{arguments: []string{"auth", "status"}, name: "auth-status"},
		{arguments: []string{"auth", "logout"}, name: "auth-logout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command, err := Parse(test.arguments)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if command.Name != test.name || command.OrgID != test.orgID {
				t.Fatalf("Parse() = %+v", command)
			}
		})
	}
}

func TestParseRejectsUnsupportedMode(t *testing.T) {
	_, err := Parse([]string{"mcp", "--mode", "automatic"})
	if err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("Parse() error = %v, want mode error", err)
	}
}

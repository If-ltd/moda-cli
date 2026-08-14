package cli

import (
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

func TestParseRejectsUnsupportedMode(t *testing.T) {
	_, err := Parse([]string{"mcp", "--mode", "automatic"})
	if err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("Parse() error = %v, want mode error", err)
	}
}

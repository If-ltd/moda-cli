package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRootRoutesClientCommands(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		want      Command
	}{
		{
			name:      "tools defaults to client mode",
			arguments: []string{"tools", "--connection-file", "/tmp/moda.json"},
			want:      Command{Name: "tools", Mode: "client", ConnectionFile: "/tmp/moda.json"},
		},
		{
			name:      "call parses its tool and JSON input",
			arguments: []string{"call", "moda_store_list", "--input", `{"page":2}`},
			want: Command{
				Name: "call", Mode: "client", ToolName: "moda_store_list",
				Arguments: map[string]any{"page": float64(2)},
			},
		},
		{
			name:      "direct auth command remains reserved",
			arguments: []string{"auth", "use-org", "3", "--session-file", "/tmp/direct.json"},
			want:      Command{Name: "auth-use-org", OrgID: "3", SessionFile: "/tmp/direct.json"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got Command
			root := NewRoot(func(_ context.Context, command Command) error {
				got = command
				return nil
			})
			root.SetArgs(test.arguments)
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("ExecuteContext() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("command = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestRootGeneratesHelpAndMarksDirectModeUnavailable(t *testing.T) {
	var output bytes.Buffer
	called := false
	root := NewRoot(func(context.Context, Command) error {
		called = true
		return nil
	})
	root.SetArgs([]string{"--help"})
	root.SetOut(&output)
	root.SetErr(&output)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if called {
		t.Fatal("help invoked a command handler")
	}
	for _, expected := range []string{"mcp", "tools", "call", "auth", "not implemented"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("help does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestRootRejectsConflictingCallInputs(t *testing.T) {
	root := NewRoot(func(context.Context, Command) error { return nil })
	root.SetArgs([]string{"call", "moda_store_list", "--input", `{}`, "--input-file", "/tmp/input.json"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	err := root.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "only one") {
		t.Fatalf("ExecuteContext() error = %v, want conflicting input error", err)
	}
}

func TestRootReadsCallInputFile(t *testing.T) {
	inputFile := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(inputFile, []byte(`{"templateId":"template-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var got Command
	root := NewRoot(func(_ context.Context, command Command) error {
		got = command
		return nil
	})
	root.SetArgs([]string{"call", "moda_product_query", "--input-file", inputFile})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	want := map[string]any{"templateId": "template-1"}
	if !reflect.DeepEqual(got.Arguments, want) {
		t.Fatalf("arguments = %#v, want %#v", got.Arguments, want)
	}
}

func TestRootRejectsUnsupportedMode(t *testing.T) {
	root := NewRoot(func(context.Context, Command) error { return nil })
	root.SetArgs([]string{"tools", "--mode", "automatic"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	err := root.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("ExecuteContext() error = %v, want mode error", err)
	}
}

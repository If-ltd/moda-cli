package modacli_test

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClientModeRunsStdioBridgeWithExplicitConnectionFile(t *testing.T) {
	fixture := newClientFixture(t)
	command := exec.Command("go", "run", "./cmd/moda-cli", "mcp", "--mode", "client", "--connection-file", fixture.connectionFile)
	command.Env = append(os.Environ(), "MODA_CLIENT_MCP_CONNECTION_FILE="+filepath.Join(fixture.directory, "wrong.json"))
	agent := mcp.NewClient(&mcp.Implementation{Name: "stdio-integration-agent", Version: "1.0.0"}, nil)
	session, err := agent.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect to moda-cli stdio bridge: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools through stdio: %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "moda_product_query" {
		t.Fatalf("stdio tools = %+v", tools.Tools)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "moda_product_query",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool through stdio: %v", err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["tool"] != "moda_product_query" {
		t.Fatalf("stdio structured result = %#v", result.StructuredContent)
	}
}

func TestClientToolsListsElectronToolsOnce(t *testing.T) {
	fixture := newClientFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", "./cmd/moda-cli", "tools", "--mode", "client", "--connection-file", fixture.connectionFile)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("moda-cli tools failed: %v\n%s", err, output)
	}
	var tools []*mcp.Tool
	if err := json.Unmarshal(output, &tools); err != nil {
		t.Fatalf("decode tools output: %v\n%s", err, output)
	}
	if len(tools) != 1 || tools[0].Name != "moda_product_query" {
		t.Fatalf("tools output = %+v", tools)
	}
}

func TestClientCallInvokesElectronToolOnce(t *testing.T) {
	fixture := newClientFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(
		ctx, "go", "run", "./cmd/moda-cli", "call", "moda_product_query",
		"--mode", "client", "--input", `{"page":2}`, "--connection-file", fixture.connectionFile,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("moda-cli call failed: %v\n%s", err, output)
	}
	var result mcp.CallToolResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode call output: %v\n%s", err, output)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["tool"] != "moda_product_query" || structured["page"] != float64(2) {
		t.Fatalf("call output = %#v", result.StructuredContent)
	}
}

type clientFixture struct {
	connectionFile string
	directory      string
}

func newClientFixture(t *testing.T) clientFixture {
	t.Helper()
	const accessToken = "stdio-bridge-token"
	remoteServer := mcp.NewServer(&mcp.Implementation{Name: "electron-integration-test", Version: "1.0.0"}, nil)
	remoteServer.AddTool(&mcp.Tool{
		Name:        "moda_product_query",
		Description: "Query products through the signed-in client.",
		InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var arguments map[string]any
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "queried"}},
			StructuredContent: map[string]any{
				"tool": request.Params.Name,
				"page": arguments["page"],
			},
		}, nil
	})
	streamableHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return remoteServer },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	mux := http.NewServeMux()
	mux.Handle("/agent-runtime/v1/mcp/client", http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		streamableHandler.ServeHTTP(response, request)
	}))
	tlsServer := httptest.NewTLSServer(mux)
	t.Cleanup(tlsServer.Close)

	directory := t.TempDir()
	rootCAPath := filepath.Join(directory, "root-ca.pem")
	rootCA := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsServer.Certificate().Raw})
	if err := os.WriteFile(rootCAPath, rootCA, 0o600); err != nil {
		t.Fatal(err)
	}
	connectionFile := filepath.Join(directory, "connection.json")
	descriptor := map[string]any{
		"protocol":         "moda.client-mcp-connection",
		"protocolVersion":  1,
		"transport":        "streamable-http",
		"endpoint":         tlsServer.URL + "/agent-runtime/v1/mcp/client",
		"localAccessToken": accessToken,
		"rootCaPath":       rootCAPath,
		"expiresAt":        time.Now().Add(5 * time.Minute).Format(time.RFC3339Nano),
		"client": map[string]any{
			"pid":       os.Getpid(),
			"startedAt": time.Now().Add(-time.Minute).Format(time.RFC3339Nano),
		},
	}
	descriptorJSON, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(connectionFile, descriptorJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(connectionFile, 0o600); err != nil {
		t.Fatal(err)
	}
	return clientFixture{connectionFile: connectionFile, directory: directory}
}

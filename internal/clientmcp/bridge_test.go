package clientmcp

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/if-ltd/moda-cli/internal/clientconn"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRunBridgeForwardsToolsAndCallsOverAuthenticatedHTTPS(t *testing.T) {
	const accessToken = "electron-process-token"
	var authenticatedRequests atomic.Int64
	remoteServer := mcp.NewServer(&mcp.Implementation{Name: "moda-electron-test", Version: "1.0.0"}, nil)
	remoteServer.AddTool(&mcp.Tool{
		Name:        "moda_3d_try_on_generate",
		Description: "Generate a design in the running Moda client.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"productTemplateId": map[string]any{"type": "string"},
			},
			"required": []string{"productTemplateId"},
		},
	}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var arguments map[string]any
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "completed"}},
			StructuredContent: map[string]any{
				"forwardedTemplateId": arguments["productTemplateId"],
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
			http.Error(response, "missing process-local authorization", http.StatusUnauthorized)
			return
		}
		authenticatedRequests.Add(1)
		streamableHandler.ServeHTTP(response, request)
	}))
	tlsServer := httptest.NewTLSServer(mux)
	t.Cleanup(tlsServer.Close)

	rootCAPath := writeTestRootCA(t, tlsServer.Certificate())
	descriptor := clientconn.Descriptor{
		Protocol:         clientconn.Protocol,
		ProtocolVersion:  clientconn.ProtocolVersion,
		Transport:        clientconn.TransportStreamableHTTP,
		Endpoint:         tlsServer.URL + "/agent-runtime/v1/mcp/client",
		LocalAccessToken: accessToken,
		RootCAPath:       rootCAPath,
		ExpiresAt:        time.Now().Add(time.Minute),
		Client: clientconn.ClientProcess{
			PID:       os.Getpid(),
			StartedAt: time.Now().Add(-time.Minute),
		},
	}

	bridgeTransport, agentTransport := mcp.NewInMemoryTransports()
	bridgeContext, cancelBridge := context.WithCancel(context.Background())
	bridgeDone := make(chan error, 1)
	go func() {
		bridgeDone <- RunBridge(bridgeContext, descriptor, bridgeTransport)
	}()

	agent := mcp.NewClient(&mcp.Implementation{Name: "bridge-test-agent", Version: "1.0.0"}, nil)
	agentSession, err := agent.Connect(context.Background(), agentTransport, nil)
	if err != nil {
		cancelBridge()
		t.Fatalf("connect to bridge: %v", err)
	}

	tools, err := agentSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list bridged tools: %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "moda_3d_try_on_generate" {
		t.Fatalf("bridged tools = %+v", tools.Tools)
	}
	result, err := agentSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "moda_3d_try_on_generate",
		Arguments: map[string]any{
			"productTemplateId": "template-1",
		},
	})
	if err != nil {
		t.Fatalf("call bridged tool: %v", err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["forwardedTemplateId"] != "template-1" {
		t.Fatalf("structured result = %#v", result.StructuredContent)
	}
	if authenticatedRequests.Load() < 3 {
		t.Fatalf("authenticated request count = %d, want initialize, tools/list, and tools/call", authenticatedRequests.Load())
	}

	if err := agentSession.Close(); err != nil {
		t.Fatalf("close agent session: %v", err)
	}
	cancelBridge()
	select {
	case err := <-bridgeDone:
		if err != nil {
			t.Fatalf("RunBridge() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunBridge() did not stop after cancellation")
	}
}

func TestConnectDesktopRejectsRedirectWithoutForwardingToken(t *testing.T) {
	const accessToken = "must-not-follow-redirect"
	var redirectedAuthorization atomic.Value
	redirectedAuthorization.Store("")
	remoteServer := mcp.NewServer(&mcp.Implementation{Name: "redirect-target", Version: "1.0.0"}, nil)
	streamableHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return remoteServer },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/agent-runtime/v1/mcp/client", func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, "/redirect-target", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/redirect-target", func(response http.ResponseWriter, request *http.Request) {
		redirectedAuthorization.Store(request.Header.Get("Authorization"))
		streamableHandler.ServeHTTP(response, request)
	})
	tlsServer := httptest.NewTLSServer(mux)
	t.Cleanup(tlsServer.Close)

	descriptor := clientconn.Descriptor{
		Endpoint:         tlsServer.URL + "/agent-runtime/v1/mcp/client",
		LocalAccessToken: accessToken,
		RootCAPath:       writeTestRootCA(t, tlsServer.Certificate()),
	}
	session, closeHTTP, err := connectDesktop(context.Background(), descriptor)
	if session != nil {
		session.Close()
	}
	if closeHTTP != nil {
		closeHTTP()
	}
	if err == nil {
		t.Fatal("connectDesktop() succeeded through an unexpected redirect")
	}
	if got := redirectedAuthorization.Load().(string); got != "" {
		t.Fatalf("redirect target received Authorization header %q", got)
	}
}

func writeTestRootCA(t *testing.T, certificate *x509.Certificate) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "root-ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return filePath
}

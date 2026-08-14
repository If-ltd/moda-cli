package clientmcp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/if-ltd/moda-cli/internal/clientconn"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func RunBridge(ctx context.Context, descriptor clientconn.Descriptor, agentTransport mcp.Transport) error {
	client, err := ConnectDesktop(ctx, descriptor)
	if err != nil {
		return err
	}
	defer client.Close()

	tools, err := client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("list tools from Moda desktop client: %w", err)
	}
	bridge := mcp.NewServer(&mcp.Implementation{Name: "moda-client-bridge", Version: "0.1.0"}, nil)
	registered := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool == nil || tool.Name == "" || tool.InputSchema == nil {
			return errors.New("Moda desktop client returned a tool without a name or input schema")
		}
		if _, exists := registered[tool.Name]; exists {
			return fmt.Errorf("Moda desktop client returned duplicate tool %q", tool.Name)
		}
		registered[tool.Name] = struct{}{}
		remoteTool := tool
		bridge.AddTool(remoteTool, func(callContext context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return client.session.CallTool(callContext, &mcp.CallToolParams{
				Meta:      request.Params.Meta,
				Name:      remoteTool.Name,
				Arguments: request.Params.Arguments,
			})
		})
	}

	if err := bridge.Run(ctx, agentTransport); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("run client MCP bridge: %w", err)
	}
	return nil
}

type DesktopClient struct {
	session   *mcp.ClientSession
	closeHTTP func()
}

func ConnectDesktop(ctx context.Context, descriptor clientconn.Descriptor) (*DesktopClient, error) {
	session, closeHTTP, err := connectDesktop(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	return &DesktopClient{session: session, closeHTTP: closeHTTP}, nil
}

func (client *DesktopClient) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	tools, err := listAllTools(ctx, client.session)
	if err != nil {
		return nil, fmt.Errorf("list tools from Moda desktop client: %w", err)
	}
	return tools, nil
}

func (client *DesktopClient) CallTool(ctx context.Context, name string, arguments any) (*mcp.CallToolResult, error) {
	result, err := client.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("call Moda desktop client tool %q: %w", name, err)
	}
	return result, nil
}

func (client *DesktopClient) Close() error {
	err := client.session.Close()
	client.closeHTTP()
	return err
}

func connectDesktop(ctx context.Context, descriptor clientconn.Descriptor) (*mcp.ClientSession, func(), error) {
	certificateData, err := os.ReadFile(descriptor.RootCAPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read Moda desktop root CA %q: %w", descriptor.RootCAPath, err)
	}
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, nil, fmt.Errorf("load system root certificates: %w", err)
	}
	if !rootCAs.AppendCertsFromPEM(certificateData) {
		return nil, nil, fmt.Errorf("Moda desktop root CA %q contains no certificates", descriptor.RootCAPath)
	}

	httpTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    rootCAs,
		},
	}
	httpClient := &http.Client{
		Transport: &authorizationTransport{
			token: descriptor.LocalAccessToken,
			base:  httpTransport,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("Moda desktop MCP endpoint must not redirect")
		},
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             descriptor.Endpoint,
		HTTPClient:           httpClient,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "moda-cli", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		httpTransport.CloseIdleConnections()
		return nil, nil, fmt.Errorf("connect to Moda desktop client MCP endpoint: %w", err)
	}
	return session, httpTransport.CloseIdleConnections, nil
}

func listAllTools(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Tool, error) {
	var tools []*mcp.Tool
	seenCursors := make(map[string]struct{})
	cursor := ""
	for {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		tools = append(tools, result.Tools...)
		if result.NextCursor == "" {
			return tools, nil
		}
		if _, seen := seenCursors[result.NextCursor]; seen {
			return nil, fmt.Errorf("Moda desktop client repeated tools/list cursor %q", result.NextCursor)
		}
		seenCursors[result.NextCursor] = struct{}{}
		cursor = result.NextCursor
	}
}

type authorizationTransport struct {
	token string
	base  http.RoundTripper
}

func (transport *authorizationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	requestCopy := request.Clone(request.Context())
	requestCopy.Header = request.Header.Clone()
	requestCopy.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.base.RoundTrip(requestCopy)
}

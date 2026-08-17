package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/if-ltd/moda-cli/internal/cli"
	"github.com/if-ltd/moda-cli/internal/clientconn"
	"github.com/if-ltd/moda-cli/internal/clientmcp"
	"github.com/if-ltd/moda-cli/internal/direct"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	root := cli.NewRoot(executeCommand)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		failure := &commandFailure{code: "invalid_arguments", status: 2, err: err}
		var classified *commandFailure
		if errors.As(err, &classified) {
			failure = classified
		}
		exitWithError(failure.code, failure.err, failure.status)
	}
}

type commandFailure struct {
	code   string
	status int
	err    error
}

func (failure *commandFailure) Error() string {
	return failure.err.Error()
}

func (failure *commandFailure) Unwrap() error {
	return failure.err
}

func executeCommand(ctx context.Context, command cli.Command) error {
	if err := directModeAvailabilityError(command); err != nil {
		return &commandFailure{code: "not_implemented", status: 1, err: err}
	}
	if strings.HasPrefix(command.Name, "auth-") {
		if err := runDirectAuth(ctx, command); err != nil {
			return &commandFailure{code: "moda_cli_failed", status: 1, err: err}
		}
		return nil
	}
	if command.Mode == "direct" {
		if command.Name != "mcp" && command.Name != "tools" && command.Name != "call" {
			return &commandFailure{
				code: "invalid_arguments", status: 2,
				err: fmt.Errorf("direct %s is not supported", command.Name),
			}
		}
		if err := runDirectCommand(ctx, command); err != nil {
			return &commandFailure{code: "moda_cli_failed", status: 1, err: err}
		}
		return nil
	}
	if err := runClientCommand(ctx, command); err != nil {
		return &commandFailure{code: "moda_cli_failed", status: 1, err: err}
	}
	return nil
}

func runClientCommand(ctx context.Context, command cli.Command) error {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	connectionFile, err := clientconn.ResolveFilePath(command.ConnectionFile, map[string]string{
		"MODA_CLIENT_MCP_CONNECTION_FILE": os.Getenv("MODA_CLIENT_MCP_CONNECTION_FILE"),
		"APPDATA":                         os.Getenv("APPDATA"),
		"XDG_CONFIG_HOME":                 os.Getenv("XDG_CONFIG_HOME"),
	}, homeDirectory, runtime.GOOS)
	if err != nil {
		return err
	}
	if command.Name == "tools" || command.Name == "call" {
		client, err := clientmcp.ConnectDesktopFile(ctx, connectionFile)
		if err != nil {
			return err
		}
		defer client.Close()
		var output any
		if command.Name == "tools" {
			output, err = client.ListTools(ctx)
		} else {
			arguments, ok := command.Arguments.(map[string]any)
			if !ok {
				arguments = map[string]any{}
			}
			output, err = client.CallTool(ctx, command.ToolName, arguments)
		}
		if err != nil {
			return err
		}
		if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
			return fmt.Errorf("encode output: %w", err)
		}
		return nil
	}
	return clientmcp.RunBridgeFile(ctx, connectionFile, &mcp.StdioTransport{})
}

func directModeAvailabilityError(command cli.Command) error {
	if command.Mode == "direct" || strings.HasPrefix(command.Name, "auth-") {
		return errors.New("direct mode is not implemented; use client mode with a running Moda desktop client")
	}
	return nil
}

func exitWithError(code string, err error, status int) {
	_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"error": code, "message": err.Error()})
	os.Exit(status)
}

func runDirectCommand(ctx context.Context, command cli.Command) error {
	store, err := directSessionStore(command.SessionFile)
	if err != nil {
		return err
	}
	client, err := direct.ConnectMCP(ctx, direct.NewAPIClient(store, nil))
	if err != nil {
		return err
	}
	defer client.Close()
	if command.Name == "mcp" {
		return client.RunBridge(ctx, &mcp.StdioTransport{})
	}
	var output any
	if command.Name == "tools" {
		output, err = client.ListTools(ctx)
	} else {
		arguments, ok := command.Arguments.(map[string]any)
		if !ok {
			arguments = map[string]any{}
		}
		output, err = client.CallTool(ctx, command.ToolName, arguments)
	}
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}
	return nil
}

func runDirectAuth(ctx context.Context, command cli.Command) error {
	store, err := directSessionStore(command.SessionFile)
	if err != nil {
		return err
	}
	client := direct.NewAPIClient(store, nil)
	var output any
	switch command.Name {
	case "auth-login":
		password, err := io.ReadAll(io.LimitReader(os.Stdin, 64*1024))
		if err != nil {
			return fmt.Errorf("read password from stdin: %w", err)
		}
		if _, err := client.Login(ctx, command.APIBaseURL, command.Account, strings.TrimRight(string(password), "\r\n")); err != nil {
			return err
		}
		output = map[string]any{"loggedIn": true, "organizationSelected": false}
	case "auth-orgs":
		output, err = client.ListOrganizations(ctx)
	case "auth-use-org":
		output, err = client.SelectOrganization(ctx, command.OrgID)
	case "auth-status":
		var session direct.Session
		session, err = store.Load()
		if err == nil {
			var expiresAtMS any
			if !session.ExpiresAt.IsZero() {
				expiresAtMS = session.ExpiresAt.UnixMilli()
			}
			output = map[string]any{
				"loggedIn": true, "apiBaseUrl": session.APIBaseURL, "orgId": nullable(session.OrgID),
				"userId": nullable(session.UserID), "expiresAtMs": expiresAtMS,
			}
		}
	case "auth-logout":
		err = store.Clear()
		output = map[string]any{"loggedIn": false}
	default:
		return fmt.Errorf("unsupported auth command %q", command.Name)
	}
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}
	return nil
}

func directSessionStore(explicit string) (*direct.SessionStore, error) {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	sessionFile, err := direct.ResolveSessionFilePath(explicit, map[string]string{
		"MODA_CLIENT_CLI_SESSION_FILE": os.Getenv("MODA_CLIENT_CLI_SESSION_FILE"),
		"APPDATA":                      os.Getenv("APPDATA"),
		"XDG_CONFIG_HOME":              os.Getenv("XDG_CONFIG_HOME"),
	}, homeDirectory, runtime.GOOS)
	if err != nil {
		return nil, err
	}
	return direct.NewSessionStore(sessionFile), nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

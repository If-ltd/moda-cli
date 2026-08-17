package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type Command struct {
	Name           string
	Mode           string
	ConnectionFile string
	SessionFile    string
	ToolName       string
	Arguments      any
	APIBaseURL     string
	Account        string
	PasswordStdin  bool
	OrgID          string
}

type Handler func(context.Context, Command) error

func NewRoot(handler Handler) *cobra.Command {
	root := &cobra.Command{
		Use:           "moda-cli",
		Short:         "Expose Moda capabilities to agents through MCP",
		Long:          "moda-cli exposes Moda capabilities to agents through MCP. Direct mode commands are reserved but not implemented.",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(
		newModeCommand("mcp", "Bridge Moda capabilities over stdio MCP", handler),
		newModeCommand("tools", "List Moda MCP tools", handler),
		newCallCommand(handler),
		newAuthCommand(handler),
	)
	return root
}

type modeOptions struct {
	mode           string
	connectionFile string
	sessionFile    string
}

func (options *modeOptions) addFlags(command *cobra.Command) {
	command.Flags().StringVar(&options.mode, "mode", "client", "capability mode: client or reserved direct")
	command.Flags().StringVar(&options.connectionFile, "connection-file", "", "override the Electron connection descriptor")
	command.Flags().StringVar(&options.sessionFile, "session-file", "", "override the reserved direct-mode session file")
}

func (options *modeOptions) command(name string) (Command, error) {
	mode := strings.TrimSpace(options.mode)
	if mode != "client" && mode != "direct" {
		return Command{}, errors.New("--mode must be client or direct")
	}
	return Command{
		Name:           name,
		Mode:           mode,
		ConnectionFile: strings.TrimSpace(options.connectionFile),
		SessionFile:    strings.TrimSpace(options.sessionFile),
	}, nil
}

func newModeCommand(name string, description string, handler Handler) *cobra.Command {
	options := modeOptions{}
	command := &cobra.Command{
		Use:   name,
		Short: description,
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			request, err := options.command(name)
			if err != nil {
				return err
			}
			return handler(command.Context(), request)
		},
	}
	options.addFlags(command)
	return command
}

func newCallCommand(handler Handler) *cobra.Command {
	options := modeOptions{}
	var inlineInput string
	var inputFile string
	command := &cobra.Command{
		Use:   "call <tool-name>",
		Short: "Call one Moda MCP tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, arguments []string) error {
			request, err := options.command("call")
			if err != nil {
				return err
			}
			request.ToolName = arguments[0]
			request.Arguments, err = parseInput(inlineInput, inputFile)
			if err != nil {
				return err
			}
			return handler(command.Context(), request)
		},
	}
	options.addFlags(command)
	command.Flags().StringVar(&inlineInput, "input", "", "tool input as JSON")
	command.Flags().StringVar(&inputFile, "input-file", "", "read tool input JSON from a file")
	return command
}

func parseInput(inline string, filePath string) (any, error) {
	inline = strings.TrimSpace(inline)
	filePath = strings.TrimSpace(filePath)
	if inline != "" && filePath != "" {
		return nil, errors.New("use only one of --input and --input-file")
	}
	data := []byte("{}")
	if inline != "" {
		data = []byte(inline)
	} else if filePath != "" {
		var err error
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read tool input file %q: %w", filePath, err)
		}
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parse tool input JSON: %w", err)
	}
	return value, nil
}

func newAuthCommand(handler Handler) *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Manage direct-mode authentication (not implemented)",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	auth.AddCommand(
		newAuthLoginCommand(handler),
		newAuthSessionCommand(handler, "orgs", "List organizations for direct mode"),
		newAuthUseOrgCommand(handler),
		newAuthSessionCommand(handler, "status", "Show direct-mode authentication status"),
		newAuthSessionCommand(handler, "logout", "Clear direct-mode authentication"),
	)
	return auth
}

func newAuthLoginCommand(handler Handler) *cobra.Command {
	var apiBaseURL string
	var account string
	var passwordStdin bool
	var sessionFile string
	command := &cobra.Command{
		Use:   "login",
		Short: "Log in for direct mode (not implemented)",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return handler(command.Context(), Command{
				Name:          "auth-login",
				APIBaseURL:    strings.TrimSpace(apiBaseURL),
				Account:       strings.TrimSpace(account),
				PasswordStdin: passwordStdin,
				SessionFile:   strings.TrimSpace(sessionFile),
			})
		},
	}
	command.Flags().StringVar(&apiBaseURL, "api-base-url", "", "Moda API base URL")
	command.Flags().StringVar(&account, "account", "", "Moda account")
	command.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin")
	command.Flags().StringVar(&sessionFile, "session-file", "", "override the direct-mode session file")
	return command
}

func newAuthSessionCommand(handler Handler, name string, description string) *cobra.Command {
	var sessionFile string
	command := &cobra.Command{
		Use:   name,
		Short: description + " (not implemented)",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return handler(command.Context(), Command{
				Name:        "auth-" + name,
				SessionFile: strings.TrimSpace(sessionFile),
			})
		},
	}
	command.Flags().StringVar(&sessionFile, "session-file", "", "override the direct-mode session file")
	return command
}

func newAuthUseOrgCommand(handler Handler) *cobra.Command {
	var sessionFile string
	command := &cobra.Command{
		Use:   "use-org <org-id>",
		Short: "Select a direct-mode organization (not implemented)",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, arguments []string) error {
			return handler(command.Context(), Command{
				Name:        "auth-use-org",
				OrgID:       strings.TrimSpace(arguments[0]),
				SessionFile: strings.TrimSpace(sessionFile),
			})
		},
	}
	command.Flags().StringVar(&sessionFile, "session-file", "", "override the direct-mode session file")
	return command
}

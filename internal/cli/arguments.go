package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
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

var valueOptions = map[string]struct{}{
	"mode": {}, "input": {}, "input-file": {}, "api-base-url": {}, "account": {},
	"connection-file": {}, "session-file": {},
}

func Parse(arguments []string) (Command, error) {
	if len(arguments) == 0 || arguments[0] == "help" || arguments[0] == "--help" || arguments[0] == "-h" {
		return Command{Name: "help"}, nil
	}
	positionals, options, err := parseTokens(arguments)
	if err != nil {
		return Command{}, err
	}
	command := positionals[0]
	mode := strings.TrimSpace(options["mode"])
	if mode == "" {
		mode = "client"
	}
	if mode != "client" && mode != "direct" {
		return Command{}, errors.New("--mode must be client or direct")
	}
	fileFields := Command{
		Mode:           mode,
		ConnectionFile: strings.TrimSpace(options["connection-file"]),
		SessionFile:    strings.TrimSpace(options["session-file"]),
	}

	switch command {
	case "mcp", "tools":
		if len(positionals) != 1 {
			return Command{}, fmt.Errorf("%s does not accept positional arguments", command)
		}
		fileFields.Name = command
		return fileFields, nil
	case "call":
		if len(positionals) != 2 {
			return Command{}, errors.New("call requires exactly one tool name")
		}
		input, err := parseInput(options)
		if err != nil {
			return Command{}, err
		}
		fileFields.Name = "call"
		fileFields.ToolName = positionals[1]
		fileFields.Arguments = input
		return fileFields, nil
	case "auth":
		return parseAuth(positionals, options)
	default:
		return Command{}, fmt.Errorf("unknown command %q", command)
	}
}

func parseTokens(arguments []string) ([]string, map[string]string, error) {
	var positionals []string
	options := make(map[string]string)
	for index := 0; index < len(arguments); index++ {
		value := arguments[index]
		if !strings.HasPrefix(value, "--") {
			positionals = append(positionals, value)
			continue
		}
		name := strings.TrimPrefix(value, "--")
		if name == "password-stdin" {
			options[name] = "true"
			continue
		}
		if _, supported := valueOptions[name]; !supported {
			return nil, nil, fmt.Errorf("unknown option --%s", name)
		}
		if index+1 >= len(arguments) || strings.HasPrefix(arguments[index+1], "--") {
			return nil, nil, fmt.Errorf("option --%s requires a value", name)
		}
		options[name] = arguments[index+1]
		index++
	}
	if len(positionals) == 0 {
		return nil, nil, errors.New("command is required")
	}
	return positionals, options, nil
}

func parseInput(options map[string]string) (any, error) {
	inline := strings.TrimSpace(options["input"])
	inputFile := strings.TrimSpace(options["input-file"])
	if inline != "" && inputFile != "" {
		return nil, errors.New("use only one of --input and --input-file")
	}
	input := []byte("{}")
	if inline != "" {
		input = []byte(inline)
	} else if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return nil, fmt.Errorf("read tool input file %q: %w", inputFile, err)
		}
		input = data
	}
	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		return nil, fmt.Errorf("parse tool input JSON: %w", err)
	}
	return value, nil
}

func parseAuth(positionals []string, options map[string]string) (Command, error) {
	if len(positionals) < 2 {
		return Command{}, errors.New("auth subcommand is required")
	}
	sessionFile := strings.TrimSpace(options["session-file"])
	switch positionals[1] {
	case "login":
		if len(positionals) != 2 {
			return Command{}, errors.New("auth login does not accept positional arguments")
		}
		apiBaseURL := strings.TrimSpace(options["api-base-url"])
		account := strings.TrimSpace(options["account"])
		if apiBaseURL == "" {
			return Command{}, errors.New("option --api-base-url is required")
		}
		if account == "" {
			return Command{}, errors.New("option --account is required")
		}
		if options["password-stdin"] != "true" {
			return Command{}, errors.New("auth login requires --password-stdin")
		}
		return Command{
			Name: "auth-login", APIBaseURL: apiBaseURL, Account: account,
			PasswordStdin: true, SessionFile: sessionFile,
		}, nil
	case "use-org":
		if len(positionals) != 3 || strings.TrimSpace(positionals[2]) == "" {
			return Command{}, errors.New("auth use-org requires exactly one organization id")
		}
		return Command{Name: "auth-use-org", OrgID: strings.TrimSpace(positionals[2]), SessionFile: sessionFile}, nil
	case "orgs", "status", "logout":
		if len(positionals) != 2 {
			return Command{}, fmt.Errorf("auth %s does not accept positional arguments", positionals[1])
		}
		return Command{Name: "auth-" + positionals[1], SessionFile: sessionFile}, nil
	default:
		return Command{}, fmt.Errorf("unknown auth subcommand %q", positionals[1])
	}
}

package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

type Command struct {
	Name           string
	Mode           string
	ConnectionFile string
}

func Parse(arguments []string) (Command, error) {
	if len(arguments) == 0 {
		return Command{}, errors.New("command is required")
	}
	if arguments[0] != "mcp" {
		return Command{}, fmt.Errorf("unknown command %q", arguments[0])
	}

	flags := flag.NewFlagSet("moda-cli mcp", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	mode := flags.String("mode", "", "connection mode")
	connectionFile := flags.String("connection-file", "", "Electron connection descriptor")
	if err := flags.Parse(arguments[1:]); err != nil {
		return Command{}, fmt.Errorf("parse mcp arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return Command{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *mode != "client" && *mode != "direct" {
		return Command{}, errors.New("--mode must be client or direct")
	}
	if *mode == "direct" && strings.TrimSpace(*connectionFile) != "" {
		return Command{}, errors.New("--connection-file is only valid in client mode")
	}
	return Command{
		Name:           "mcp",
		Mode:           *mode,
		ConnectionFile: strings.TrimSpace(*connectionFile),
	}, nil
}

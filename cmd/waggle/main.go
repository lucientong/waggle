// Command waggle is the command-line interface for the Waggle orchestration engine.
//
// Usage:
//
//	waggle run workflow.yaml       Run a workflow from a YAML definition
//	waggle serve [--addr :8080]    Start the web visualization panel
//	waggle validate workflow.yaml  Validate a workflow YAML file
//	waggle dot workflow.yaml       Export the DAG as Graphviz DOT format
//	waggle version                 Print version information
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/lucientong/waggle/cmd/waggle/commands"
)

const usageText = `Waggle — Lightweight AI Agent Orchestration Engine 🐝

Usage:
  waggle <command> [options]

Commands:
  run       <workflow.yaml>  Execute a workflow from a YAML definition
  serve                      Start the web visualization panel
  validate  <workflow.yaml>  Validate a workflow YAML file without running it
  dot       <workflow.yaml>  Output the DAG in Graphviz DOT format
  version                    Print version information

Options:
  -h, --help    Show this help message

Examples:
  waggle run examples/code_review/workflow.yaml
  waggle serve --addr :8080
  waggle validate my_workflow.yaml
  waggle dot my_workflow.yaml | dot -Tpng -o dag.png
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(usageText)
		return nil
	}

	cmd := args[0]
	rest := args[1:]

	switch strings.ToLower(cmd) {
	case "run":
		return commands.Run(ctx, rest)
	case "serve":
		return commands.Serve(ctx, rest)
	case "validate":
		return commands.Validate(ctx, rest)
	case "dot":
		return commands.Dot(ctx, rest)
	case "version":
		return commands.Version()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", cmd)
		fmt.Print(usageText)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

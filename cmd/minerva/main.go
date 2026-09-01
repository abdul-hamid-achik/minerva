// Command minerva is a CLI and MCP server that reads agent-harness
// conversations, proposes skills, and syncs SKILL.md libraries.
package main

import (
	"fmt"
	"os"

	"github.com/abdul-hamid-achik/minerva/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		if code, ok := cli.AsExitCode(err); ok {
			os.Exit(code)
		}
		fmt.Fprintf(os.Stderr, "minerva: %v\n", err)
		os.Exit(1)
	}
}

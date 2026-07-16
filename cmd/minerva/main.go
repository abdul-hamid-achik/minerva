// Command minerva is an agent self-improvement CLI and MCP tool.
// It manages skills, agent profiles, system prompts, and monitors
// the intelligence stack (bob, cortex, mcphub, codemap, vecgrep, etc.).
package main

import (
	"fmt"
	"os"

	"github.com/abdul-hamid-achik/minerva/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "minerva: %v\n", err)
		os.Exit(1)
	}
}

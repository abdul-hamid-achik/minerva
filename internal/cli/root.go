// Package cli provides the Cobra CLI for Minerva.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
	"github.com/abdul-hamid-achik/minerva/internal/learn"
	"github.com/abdul-hamid-achik/minerva/internal/session"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/version"
)

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "minerva",
		Short: "Skill intelligence for agent harnesses",
		Long: `Minerva reads conversations and tool calls from Claude Code, Codex, Cursor,
and other harnesses, proposes SKILL.md files from those traces, and syncs
the canonical ~/.agents/skills library into each harness.`,
		Version:       version.Full(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newSkillCmd(),
		newHarnessCmd(),
		newSessionsCmd(),
		newAnalyzeCmd(),
		newProposeCmd(),
		newLearnCmd(),
		newMCPCmd(),
		newInitCmd(),
	)
	return root
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the agents directory structure",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir := agentsDir()
			for _, sub := range []string{"skills", ".minerva"} {
				if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
					return fmt.Errorf("create %s: %w", sub, err)
				}
			}
			fmt.Printf("initialized agents directory at %s\n", dir)
			return nil
		},
	}
}

func newLearnCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "learn",
		Short: "One-page onboarding brief for agents",
		RunE: func(cmd *cobra.Command, _ []string) error {
			brief := learn.Build()
			if jsonOut {
				return printJSON(brief)
			}
			fmt.Println(brief.Thesis)
			fmt.Println()
			fmt.Println(brief.How)
			fmt.Println()
			fmt.Println("Commands:")
			for _, c := range brief.Commands {
				fmt.Printf("  %s\n", c)
			}
			fmt.Println()
			fmt.Println("MCP tools:")
			for _, tool := range brief.Tools {
				fmt.Printf("  %s — %s\n", tool.Name, tool.UseWhen)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func agentsDir() string {
	if dir := os.Getenv("MINERVA_AGENTS_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".agents")
	}
	return filepath.Join(home, ".agents")
}

func env() harness.Env {
	e := harness.DefaultEnv()
	e.AgentsDir = agentsDir()
	return e
}

func skillManager() *skill.Manager {
	return skill.ForAgents(agentsDir())
}

// loadSkills loads the library and reports the skills it skipped on stderr.
func loadSkills() (*skill.Manager, error) {
	mgr := skillManager()
	if err := mgr.LoadAll(); err != nil {
		return nil, err
	}
	warnSkipped(mgr)
	return mgr, nil
}

// catalogForAnalysis loads the library for analyze and propose. There the
// catalog only feeds load-gap and update proposals, so a library that cannot
// be read is a warning, not a failure.
func catalogForAnalysis() *skill.Manager {
	mgr := skillManager()
	if err := mgr.LoadAll(); err != nil {
		fmt.Fprintf(os.Stderr, "minerva: warning: skills not loaded, so no load-gap or update proposals: %v\n", err)
	}
	warnSkipped(mgr)
	return mgr
}

func warnSkipped(mgr *skill.Manager) {
	for _, p := range mgr.Problems() {
		fmt.Fprintf(os.Stderr, "minerva: warning: skipped skill: %v\n", p)
	}
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func parseSince(s string) (time.Duration, error) {
	d, err := session.ParseSince(s)
	if err != nil {
		return 0, fmt.Errorf("--since: %w", err)
	}
	return d, nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

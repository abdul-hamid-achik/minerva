// Package cli provides the Cobra CLI for Minerva.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/abdul-hamid-achik/minerva/internal/analytics"
	"github.com/abdul-hamid-achik/minerva/internal/bridge"
	"github.com/abdul-hamid-achik/minerva/internal/evidence"
	"github.com/abdul-hamid-achik/minerva/internal/integration"
	"github.com/abdul-hamid-achik/minerva/internal/library"
	minervamcp "github.com/abdul-hamid-achik/minerva/internal/mcp"
	"github.com/abdul-hamid-achik/minerva/internal/monitor"
	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/status"
	"github.com/abdul-hamid-achik/minerva/internal/suggest"
	"github.com/abdul-hamid-achik/minerva/internal/templates"
	"github.com/abdul-hamid-achik/minerva/internal/textdiff"
	"github.com/abdul-hamid-achik/minerva/internal/version"
)

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "minerva",
		Short: "Agent library operator and stack readiness CLI/MCP",
		Long: `Minerva manages skills and profiles under ~/.agents (shared with local-agent)
and orchestrates stack presence/readiness probes for companion tools
(bob, cortex, mcphub, codemap, vecgrep, …).

Activation state is Minerva-local. local-agent loads profiles and skills from
disk into its own session — it does not read .minerva-skills.json.`,
		Version:       version.Full(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newSkillCmd(),
		newProfileCmd(),
		newStackCmd(),
		newStatusCmd(),
		newAnalyticsCmd(),
		newSuggestCmd(),
		newTemplateCmd(),
		newLibraryCmd(),
		newBridgeCmd(),
		newEvidenceCmd(),
		newMCPCmd(),
		newInitCmd(),
	)
	return root
}

// --- Skill commands ---

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage agent skills",
	}
	cmd.AddCommand(
		newSkillListCmd(),
		newSkillShowCmd(),
		newSkillCompareCmd(),
		newSkillCreateCmd(),
		newSkillUpdateCmd(),
		newSkillActivateCmd(),
		newSkillDeactivateCmd(),
		newSkillDeleteCmd(),
	)
	return cmd
}

func newSkillListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all skills",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(mgr.Catalog())
			}
			for _, s := range mgr.All() {
				active := " "
				if s.Active {
					active = "*"
				}
				fmt.Printf(" [%s] %s", active, s.Name)
				if s.Description != "" {
					fmt.Printf(" — %s", s.Description)
				}
				fmt.Println()
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func newSkillShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show a skill's full content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			content, ok := mgr.Load(args[0])
			if !ok {
				return fmt.Errorf("skill %q not found", args[0])
			}
			fmt.Println(content)
			return nil
		},
	}
	return cmd
}

func newSkillCompareCmd() *cobra.Command {
	var sideBySide bool
	cmd := &cobra.Command{
		Use:   "compare <name-a> <name-b>",
		Short: "Compare two skills (unified diff by default)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			contentA, okA := mgr.Load(args[0])
			contentB, okB := mgr.Load(args[1])
			if !okA {
				return fmt.Errorf("skill %q not found", args[0])
			}
			if !okB {
				return fmt.Errorf("skill %q not found", args[1])
			}
			if sideBySide {
				fmt.Printf("=== %s ===\n%s\n\n=== %s ===\n%s\n", args[0], contentA, args[1], contentB)
				return nil
			}
			diff := textdiff.Unified(args[0], args[1], contentA, contentB)
			if diff == "" {
				fmt.Println("skills are identical")
				return nil
			}
			fmt.Print(diff)
			return nil
		},
	}
	cmd.Flags().BoolVar(&sideBySide, "side-by-side", false, "print full bodies instead of unified diff")
	return cmd
}

func newSkillCreateCmd() *cobra.Command {
	var description, fromFile string
	cmd := &cobra.Command{
		Use:   "create <name> [content]",
		Short: "Create a new skill",
		Long:  `Create a skill under ~/.agents/skills/<name>/SKILL.md. Pass body as the second argument or via --from-file.`,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			content, err := resolveContentArg(args, 1, fromFile)
			if err != nil {
				return err
			}
			if strings.TrimSpace(content) == "" {
				return fmt.Errorf("content is required (positional argument or --from-file)")
			}
			skillsDir := filepath.Join(agentsDir(), "skills")
			if err := mgr.Create(skillsDir, args[0], description, content); err != nil {
				return err
			}
			_ = analyticsStore().Record("skill_create", args[0], description)
			fmt.Printf("skill %q created\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&description, "description", "d", "", "one-line description")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "read skill body from file")
	return cmd
}

func newSkillUpdateCmd() *cobra.Command {
	var description, fromFile, contentFlag string
	var setDescription bool
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an existing skill's description and/or body",
		Long:  `Update skill frontmatter description and/or markdown body. Use --description, --content, and/or --from-file.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			var descPtr *string
			var contentPtr *string
			if setDescription {
				descPtr = &description
			}
			if fromFile != "" {
				data, err := os.ReadFile(fromFile)
				if err != nil {
					return fmt.Errorf("read --from-file: %w", err)
				}
				s := string(data)
				contentPtr = &s
			} else if cmd.Flags().Changed("content") {
				contentPtr = &contentFlag
			}
			if descPtr == nil && contentPtr == nil {
				return fmt.Errorf("nothing to update: pass --description and/or --content/--from-file")
			}
			if err := mgr.Update(args[0], descPtr, contentPtr); err != nil {
				return err
			}
			_ = analyticsStore().Record("skill_update", args[0], "")
			fmt.Printf("skill %q updated\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&description, "description", "d", "", "new one-line description")
	cmd.Flags().StringVar(&contentFlag, "content", "", "new markdown body")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "read new body from file")
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		if cmd.Flags().Changed("description") {
			setDescription = true
		}
	}
	return cmd
}

func newSkillActivateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate <name>",
		Short: "Mark a skill active in Minerva-local catalog state",
		Long: `Updates ~/.agents/.minerva-skills.json only.

local-agent does not read this file. For durable harness behavior, add the
skill to a profile: minerva profile add-skills <profile> <skill>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if err := mgr.Activate(args[0]); err != nil {
				return err
			}
			_ = analyticsStore().Record("skill_activate", args[0], "")
			fmt.Printf("skill %q activated (Minerva-local only; use profile add-skills for local-agent)\n", args[0])
			return nil
		},
	}
	return cmd
}

func newSkillDeactivateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deactivate <name>",
		Short: "Deactivate a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if err := mgr.Deactivate(args[0]); err != nil {
				return err
			}
			_ = analyticsStore().Record("skill_deactivate", args[0], "")
			fmt.Printf("skill %q deactivated\n", args[0])
			return nil
		},
	}
	return cmd
}

func newSkillDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			skillsDir := filepath.Join(agentsDir(), "skills")
			if err := mgr.Delete(skillsDir, args[0]); err != nil {
				return err
			}
			fmt.Printf("skill %q deleted\n", args[0])
			return nil
		},
	}
	return cmd
}

// --- Profile commands ---

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage agent profiles",
	}
	cmd.AddCommand(
		newProfileListCmd(),
		newProfileShowCmd(),
		newProfileCompareCmd(),
		newProfileCreateCmd(),
		newProfileUpdatePromptCmd(),
		newProfileUpdateSkillsCmd(),
		newProfileAddSkillsCmd(),
		newProfileRemoveSkillsCmd(),
		newProfileUpdateModelCmd(),
		newProfileUpdateMCPCmd(),
		newProfileUpdateDescCmd(),
		newProfileDeleteCmd(),
	)
	return cmd
}

func newProfileListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all agent profiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]any{
					"profiles": mgr.All(),
					"warnings": mgr.Warnings(),
				}
				return printJSON(payload)
			}
			for _, w := range mgr.Warnings() {
				fmt.Fprintf(os.Stderr, "warning: profile %q — %s\n", w.Dir, w.Message)
			}
			for _, p := range mgr.All() {
				fmt.Printf("%s", p.Name)
				if p.Description != "" {
					fmt.Printf(" — %s", p.Description)
				}
				if p.Model != "" {
					fmt.Printf(" [model: %s]", p.Model)
				}
				if len(p.Skills) > 0 {
					fmt.Printf(" [skills: %s]", strings.Join(p.Skills, ", "))
				}
				fmt.Println()
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func newProfileShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show a profile's full configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			p := mgr.Get(args[0])
			if p == nil {
				return fmt.Errorf("profile %q not found", args[0])
			}
			fmt.Printf("Name:        %s\n", p.Name)
			fmt.Printf("Description: %s\n", p.Description)
			fmt.Printf("Model:       %s\n", p.Model)
			fmt.Printf("Skills:      %s\n", strings.Join(p.Skills, ", "))
			fmt.Printf("MCP Servers: %s\n", strings.Join(p.MCPServers, ", "))
			fmt.Printf("Use Cases:   %s\n", strings.Join(p.UseCases, ", "))
			if p.SystemPrompt != "" {
				fmt.Printf("\nSystem Prompt:\n%s\n", p.SystemPrompt)
			}
			return nil
		},
	}
	return cmd
}

func newProfileCompareCmd() *cobra.Command {
	var sideBySide bool
	cmd := &cobra.Command{
		Use:   "compare <name-a> <name-b>",
		Short: "Compare two profiles (unified YAML diff by default)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			pA := mgr.Get(args[0])
			pB := mgr.Get(args[1])
			if pA == nil {
				return fmt.Errorf("profile %q not found", args[0])
			}
			if pB == nil {
				return fmt.Errorf("profile %q not found", args[1])
			}
			if sideBySide {
				fmt.Printf("=== %s ===\n", args[0])
				fmt.Printf("  Model:       %s\n", pA.Model)
				fmt.Printf("  Skills:      %s\n", strings.Join(pA.Skills, ", "))
				fmt.Printf("  MCP Servers: %s\n", strings.Join(pA.MCPServers, ", "))
				fmt.Printf("  Prompt:      %s\n", truncate(pA.SystemPrompt, 80))
				fmt.Printf("\n=== %s ===\n", args[1])
				fmt.Printf("  Model:       %s\n", pB.Model)
				fmt.Printf("  Skills:      %s\n", strings.Join(pB.Skills, ", "))
				fmt.Printf("  MCP Servers: %s\n", strings.Join(pB.MCPServers, ", "))
				fmt.Printf("  Prompt:      %s\n", truncate(pB.SystemPrompt, 80))
				return nil
			}
			ya, err := profileYAML(pA)
			if err != nil {
				return err
			}
			yb, err := profileYAML(pB)
			if err != nil {
				return err
			}
			diff := textdiff.Unified(args[0]+"/agent.yaml", args[1]+"/agent.yaml", ya, yb)
			if diff == "" {
				fmt.Println("profiles are identical")
				return nil
			}
			fmt.Print(diff)
			return nil
		},
	}
	cmd.Flags().BoolVar(&sideBySide, "side-by-side", false, "print summary fields instead of unified diff")
	return cmd
}

func newProfileCreateCmd() *cobra.Command {
	var description, model string
	var skills, mcpServers []string
	cmd := &cobra.Command{
		Use:   "create <name> [system-prompt]",
		Short: "Create a new agent profile",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			prompt := ""
			if len(args) == 2 {
				prompt = args[1]
			}
			p := &profile.Profile{
				Name:         args[0],
				Description:  description,
				Model:        model,
				Skills:       skills,
				MCPServers:   mcpServers,
				SystemPrompt: prompt,
			}
			if err := mgr.Create(p); err != nil {
				return err
			}
			_ = analyticsStore().Record("profile_create", args[0], description)
			fmt.Printf("profile %q created\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&description, "description", "d", "", "one-line description")
	cmd.Flags().StringVarP(&model, "model", "m", "", "Ollama model")
	cmd.Flags().StringSliceVarP(&skills, "skill", "s", nil, "skill names (repeatable)")
	cmd.Flags().StringSliceVar(&mcpServers, "mcp-server", nil, "MCP server names (repeatable)")
	return cmd
}

func newProfileUpdatePromptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-prompt <name> <prompt>",
		Short: "Update a profile's system prompt",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if err := mgr.UpdateSystemPrompt(args[0], args[1]); err != nil {
				return err
			}
			_ = analyticsStore().Record("profile_update_prompt", args[0], "")
			fmt.Printf("system prompt updated for profile %q\n", args[0])
			return nil
		},
	}
	return cmd
}

func newProfileUpdateSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-skills <name> <skill1,skill2,...>",
		Short: "Replace a profile's skills list",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			skills := splitCSV(args[1])
			if err := mgr.UpdateSkills(args[0], skills); err != nil {
				return err
			}
			_ = analyticsStore().Record("profile_update_skills", args[0], strings.Join(skills, ","))
			fmt.Printf("skills replaced for profile %q\n", args[0])
			return nil
		},
	}
	return cmd
}

func newProfileAddSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-skills <name> <skill1,skill2,...>",
		Short: "Merge skills into a profile without dropping existing ones",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			skills := splitCSV(args[1])
			if err := mgr.AddSkills(args[0], skills); err != nil {
				return err
			}
			_ = analyticsStore().Record("profile_add_skills", args[0], strings.Join(skills, ","))
			fmt.Printf("skills added to profile %q\n", args[0])
			return nil
		},
	}
	return cmd
}

func newProfileRemoveSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-skills <name> <skill1,skill2,...>",
		Short: "Remove skills from a profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			skills := splitCSV(args[1])
			if err := mgr.RemoveSkills(args[0], skills); err != nil {
				return err
			}
			_ = analyticsStore().Record("profile_remove_skills", args[0], strings.Join(skills, ","))
			fmt.Printf("skills removed from profile %q\n", args[0])
			return nil
		},
	}
	return cmd
}

func newProfileUpdateModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-model <name> <model>",
		Short: "Update a profile's model",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if err := mgr.UpdateModel(args[0], args[1]); err != nil {
				return err
			}
			_ = analyticsStore().Record("profile_update_model", args[0], args[1])
			fmt.Printf("model updated for profile %q\n", args[0])
			return nil
		},
	}
	return cmd
}

func newProfileUpdateMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-mcp <name> <server1,server2,...>",
		Short: "Replace a profile's MCP server allowlist",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			servers := splitCSV(args[1])
			if err := mgr.UpdateMCPServers(args[0], servers); err != nil {
				return err
			}
			_ = analyticsStore().Record("profile_update_mcp", args[0], strings.Join(servers, ","))
			fmt.Printf("mcp_servers updated for profile %q\n", args[0])
			return nil
		},
	}
	return cmd
}

func newProfileUpdateDescCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-desc <name> <description>",
		Short: "Update a profile's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if err := mgr.UpdateDescription(args[0], args[1]); err != nil {
				return err
			}
			_ = analyticsStore().Record("profile_update_desc", args[0], "")
			fmt.Printf("description updated for profile %q\n", args[0])
			return nil
		},
	}
	return cmd
}

func newProfileDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an agent profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if err := mgr.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("profile %q deleted\n", args[0])
			return nil
		},
	}
	return cmd
}

// --- Status / doctor ---

func newStatusCmd() *cobra.Command {
	var workspace string
	var jsonOut, deep, noEvidence, noSuggest, watch bool
	var maxNext int
	var interval time.Duration
	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"doctor"},
		Short:   "Unified library + stack + evidence status",
		Long: `One operator view: library inventory, stack presence, optional deep readiness,
open evidence fails, and top next actions.

Exit codes (after printing; not used with --watch):
  0  healthy
  1  unhealthy (core incomplete)
  2  degraded (optional missing, retrieval, library warnings, open fails)
With --require-retrieval, also exits 3 when retrieval is not ready (implies --deep).

--watch re-probes on an interval until interrupted (Ctrl-C).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := workspace
			if ws == "" {
				ws, _ = os.Getwd()
			}
			requireRetrieval, _ := cmd.Flags().GetBool("require-retrieval")
			if requireRetrieval {
				deep = true
			}

			runOnce := func() (status.Report, error) {
				sm := skillManager()
				if err := sm.LoadAll(); err != nil {
					return status.Report{}, err
				}
				pm := profileManager()
				if err := pm.LoadAll(); err != nil {
					return status.Report{}, err
				}
				return status.Build(cmd.Context(), sm, pm, status.Options{
					Workspace:       ws,
					Deep:            deep,
					IncludeEvidence: !noEvidence,
					IncludeSuggest:  !noSuggest,
					MaxNextActions:  maxNext,
				}), nil
			}

			printRep := func(rep status.Report) error {
				if jsonOut {
					return printJSON(rep)
				}
				fmt.Print(status.FormatHuman(rep))
				return nil
			}

			if watch {
				if interval <= 0 {
					interval = 60 * time.Second
				}
				ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
				defer cancel()
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					rep, err := runOnce()
					if err != nil {
						return err
					}
					if !jsonOut {
						fmt.Fprintf(os.Stderr, "\n--- %s (next in %s; Ctrl-C to stop) ---\n",
							time.Now().Format(time.RFC3339), interval)
					}
					if err := printRep(rep); err != nil {
						return err
					}
					select {
					case <-ctx.Done():
						return nil
					case <-ticker.C:
					}
				}
			}

			rep, err := runOnce()
			if err != nil {
				return err
			}
			if err := printRep(rep); err != nil {
				return err
			}

			if requireRetrieval && rep.Deep != nil && !rep.Deep.RetrievalReady {
				return ExitCode(3)
			}
			switch rep.Verdict {
			case "unhealthy":
				return ExitCode(1)
			case "degraded":
				return ExitCode(2)
			default:
				return nil
			}
		},
	}
	cmd.Flags().StringVarP(&workspace, "workspace", "C", "", "workspace for deep/suggest probes")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&deep, "deep", true, "include stack deep readiness probes")
	cmd.Flags().BoolVar(&noEvidence, "no-evidence", false, "skip fcheap open-fail counts")
	cmd.Flags().BoolVar(&noSuggest, "no-suggest", false, "skip top next-action suggestions")
	cmd.Flags().IntVar(&maxNext, "max-next", 5, "max next actions to include")
	cmd.Flags().Bool("require-retrieval", false, "exit 3 when retrieval not ready (implies --deep)")
	cmd.Flags().BoolVar(&watch, "watch", false, "re-run status until interrupted")
	cmd.Flags().DurationVar(&interval, "interval", 60*time.Second, "watch interval (e.g. 30s, 2m)")
	return cmd
}

// --- Stack commands ---

func newStackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stack",
		Short: "Monitor the intelligence stack",
	}
	cmd.AddCommand(
		newStackCheckCmd(),
		newStackDeepCmd(),
	)
	return cmd
}

func newStackCheckCmd() *cobra.Command {
	var jsonOut, strict bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check intelligence stack presence (tiered; correct binaries)",
		Long: `Probes PATH for stack tools using real binary names (glyph, cairn, tvault).
Core tools missing → unhealthy (exit 1); optional missing → degraded.
With --strict, degraded also exits 2. Domain readiness is under stack deep.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			status := monitor.CheckStack()
			if jsonOut {
				data, err := json.MarshalIndent(status, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(data))
			} else {
				for _, tool := range status.Tools {
					icon := "✓"
					if !tool.Found {
						icon = "✗"
					}
					fmt.Printf(" %s %-12s %-10s bin=%-10s", icon, tool.Name, tool.Tier, tool.Command)
					if tool.Found {
						if tool.Version != "" {
							fmt.Printf(" %s", tool.Version)
						} else if tool.Error != "" {
							fmt.Printf(" (present; %s)", tool.Error)
						}
					} else {
						fmt.Printf(" %s", tool.Error)
					}
					fmt.Println()
				}
				fmt.Printf("\n%s\n", status.Summary)
			}
			if !status.CoreHealthy {
				return ExitCode(1)
			}
			if strict && status.Degraded {
				return ExitCode(2)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&strict, "strict", false, "exit 2 when optional/infra tools are missing")
	return cmd
}

func newStackDeepCmd() *cobra.Command {
	var workspace string
	var jsonOut, stash, requireRetrieval, requireCore bool
	cmd := &cobra.Command{
		Use:   "deep [workspace]",
		Short: "Deep stack probe (bob/cortex/mcphub + readiness doctors)",
		Long: `Composes sibling CLI contracts: bob check/context, cortex doctor, mcphub stats,
plus optional codemap/vecgrep/fcheap/tvault/monitor readiness probes.

Sets retrieval_ready only when both codemap and vecgrep are domain-ready.
--stash writes the JSON report to fcheap with minerva-stack tags (TTL 30d).

Exit codes (after printing the report):
  0  ok
  1  --require-core and core presence incomplete
  3  --require-retrieval and retrieval_ready is false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := workspace
			if ws == "" && len(args) > 0 {
				ws = args[0]
			}
			if ws == "" {
				ws = "."
			}
			status := integration.DeepCheck(cmd.Context(), ws)

			if stash {
				outcome := "pass"
				if !status.RetrievalReady {
					outcome = "fail"
				}
				extra := []string{}
				if status.RetrievalReady {
					extra = append(extra, "retrieval:ready")
				} else {
					extra = append(extra, "retrieval:not-ready")
					for _, g := range status.RetrievalGaps {
						extra = append(extra, "gap:"+g)
					}
				}
				res, err := evidence.SaveJSON(cmd.Context(), "stack-deep", "stack", outcome, extra, status)
				if err != nil {
					fmt.Fprintf(os.Stderr, "stash warning: %v\n", err)
				} else if res != nil && res.ID != "" {
					fmt.Fprintf(os.Stderr, "stashed stack deep id=%s outcome=%s\n", res.ID, outcome)
					_ = analyticsStore().Record("stack_deep_stash", res.ID, outcome)
				}
			}

			if jsonOut {
				if err := printJSON(status); err != nil {
					return err
				}
			} else {
			fmt.Printf("=== Intelligence Stack Deep Probe ===\n\n")
			if status.Bob.Error != "" {
				fmt.Printf("bob:     %s\n", status.Bob.Error)
			} else {
				fmt.Printf("bob:     recipe=%s clean=%v drift=%v", status.Bob.Recipe, status.Bob.Clean, status.Bob.Drift)
				if status.Bob.Code != "" {
					fmt.Printf(" code=%s", status.Bob.Code)
				}
				fmt.Println()
				if status.Bob.RawNote != "" {
					fmt.Printf("         note=%s\n", status.Bob.RawNote)
				}
				for _, a := range status.Bob.NextActions {
					fmt.Printf("       → %s\n", a)
				}
			}
			if status.Cortex.Error != "" {
				fmt.Printf("cortex:  %s\n", status.Cortex.Error)
			} else {
				fmt.Printf("cortex:  ready=%v source=%s\n", status.Cortex.Ready, status.Cortex.Source)
				if status.Cortex.Version != "" {
					fmt.Printf("         version=%s\n", firstLineCLI(status.Cortex.Version))
				}
				if status.Cortex.Sessions > 0 {
					fmt.Printf("         sessions=%d active=%d stale=%d completed=%d verified=%d\n",
						status.Cortex.Sessions, status.Cortex.Active, status.Cortex.Stale,
						status.Cortex.Completed, status.Cortex.Verified)
					fmt.Printf("         completion_rate=%.1f%% verified_rate=%.1f%%\n",
						status.Cortex.CompletionRate*100, status.Cortex.VerifiedRate*100)
				}
				if status.Cortex.ActiveWorkspace > 0 {
					fmt.Printf("         active_in_workspace=%d\n", status.Cortex.ActiveWorkspace)
				}
				for _, s := range status.Cortex.StaleSamples {
					fmt.Printf("         stale %s [%s] %s — %s\n", s.ID, s.Repository, s.Phase, s.Goal)
				}
			}
			if status.MCPHub.Error != "" {
				fmt.Printf("mcphub:  %s\n", status.MCPHub.Error)
			} else {
				fmt.Printf("mcphub:  calls=%d errors=%d tokens=%d servers=%d",
					status.MCPHub.TotalCalls, status.MCPHub.ErrorCount,
					status.MCPHub.EstTokens, status.MCPHub.ServerCount)
				if status.MCPHub.EnabledCount > 0 {
					fmt.Printf(" enabled=%d", status.MCPHub.EnabledCount)
				}
				fmt.Println()
				if len(status.MCPHub.TopServers) > 0 {
					fmt.Printf("         top=%s\n", strings.Join(status.MCPHub.TopServers, ", "))
				}
				if len(status.MCPHub.UnusedEnabled) > 0 {
					fmt.Printf("         unused_enabled=%s\n", strings.Join(status.MCPHub.UnusedEnabled, ", "))
				}
				if len(status.MCPHub.AgentsDrift) > 0 {
					fmt.Printf("         agents_drift=%s\n", strings.Join(status.MCPHub.AgentsDrift, ", "))
				}
			}
			if len(status.Readiness) > 0 {
				fmt.Printf("\n-- readiness --\n")
				for _, r := range status.Readiness {
					icon := "✓"
					if !r.Ready {
						icon = "✗"
					}
					fmt.Printf(" %s %-10s", icon, r.Tool)
					if r.Error != "" {
						fmt.Printf(" %s", r.Error)
					} else if r.Detail != "" {
						d := r.Detail
						if len(d) > 90 {
							d = d[:90] + "…"
						}
						fmt.Printf(" %s", d)
					}
					fmt.Println()
					for _, a := range r.NextActions {
						fmt.Printf("       → %s\n", a)
					}
				}
			}
			// Retrieval green light
			retIcon := "✓"
			if !status.RetrievalReady {
				retIcon = "✗"
			}
			fmt.Printf("\n%s retrieval_ready=%v\n", retIcon, status.RetrievalReady)
			if status.RetrievalDetail != "" {
				fmt.Printf("  %s\n", status.RetrievalDetail)
			}
			if status.MCPHub != nil && len(status.MCPHub.HighErrorServers) > 0 {
				fmt.Printf("\nmcphub high-error servers: %s\n", strings.Join(status.MCPHub.HighErrorServers, ", "))
			}
			fmt.Printf("\n%s\n", status.Summary)
			} // end !jsonOut

			if requireCore {
				presence := monitor.CheckStack()
				if !presence.CoreHealthy {
					return ExitCode(1)
				}
			}
			if requireRetrieval && !status.RetrievalReady {
				return ExitCode(3)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&workspace, "workspace", "C", "", "workspace directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&stash, "stash", false, "save report to fcheap (minerva-stack tags, TTL 30d)")
	cmd.Flags().BoolVar(&requireRetrieval, "require-retrieval", false, "exit 3 when retrieval_ready is false")
	cmd.Flags().BoolVar(&requireCore, "require-core", false, "exit 1 when core stack presence is incomplete")
	return cmd
}

// --- Analytics command ---

func newAnalyticsCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "analytics",
		Short: "View Minerva-local usage analytics",
		Long: `Shows events recorded by Minerva itself (skill activate/create, profile updates, …).

This is NOT harness telemetry: local-agent session skill loads and mcphub tool_calls
are owned by those tools. Prefer minerva status / stack deep / evidence for readiness.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := analyticsStore()
			_ = store.Load()
			summary := store.Summarize()
			if jsonOut {
				return printJSON(summary)
			}
			if summary.Note != "" {
				fmt.Printf("Note: %s\n", summary.Note)
			}
			fmt.Printf("Total events: %d\n", summary.TotalEvents)
			fmt.Printf("Suggestions applied: %d\n", summary.SuggestionsApplied)
			if !summary.LastActivity.IsZero() {
				fmt.Printf("Last activity: %s\n", summary.LastActivity.Format("2006-01-02 15:04:05"))
			}
			if len(summary.TopSkills) > 0 {
				fmt.Printf("Top skills: %s\n", strings.Join(summary.TopSkills, ", "))
			}
			if len(summary.TopProfiles) > 0 {
				fmt.Printf("Top profiles: %s\n", strings.Join(summary.TopProfiles, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

// --- Suggest command ---

func newSuggestCmd() *cobra.Command {
	var jsonOut, apply, applyLocal bool
	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "Get library and stack improvement suggestions",
		Long: `Analyze skills, profiles, stack presence, analytics, and workspace type.

Durable suggestions prefer profile skill membership (shared with local-agent).
Minerva-local activation (~/.agents/.minerva-skills.json) is secondary.

--apply runs allowlisted "minerva profile add-skills …" actions only.
--apply-local also allows "minerva skill activate <name>" (catalog pin only).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			pmgr := profileManager()
			if err := pmgr.LoadAll(); err != nil {
				return err
			}
			for _, w := range pmgr.Warnings() {
				fmt.Fprintf(os.Stderr, "warning: profile %q — %s\n", w.Dir, w.Message)
			}
			store := analyticsStore()
			_ = store.Load()

			ws, _ := os.Getwd()
			engine := suggest.NewEngine(mgr, pmgr, store, ws)
			engine.IncludeReadiness = true
			engine.IncludeEvidence = true
			suggestions := engine.Analyze()

			if apply || applyLocal {
				applied, skipped, err := suggest.ApplyAuto(mgr, pmgr, suggestions, applyLocal)
				if err != nil {
					return err
				}
				for _, name := range applied {
					fmt.Printf("applied: %s\n", name)
				}
				for _, s := range skipped {
					fmt.Printf("skipped: %s\n", s)
				}
				_ = store.Record("suggestion_applied", fmt.Sprintf("%d", len(applied)), strings.Join(applied, ","))
				fmt.Printf("\n%d applied, %d skipped\n", len(applied), len(skipped))
				return nil
			}

			if jsonOut {
				return printJSON(suggestions)
			}
			for _, s := range suggestions {
				prio := ""
				switch s.Priority {
				case 1:
					prio = "CRIT"
				case 2:
					prio = "HIGH"
				case 3:
					prio = "MED "
				case 4:
					prio = "LOW "
				}
				fmt.Printf("[%s] [%s] %s\n", prio, s.Category, s.Message)
				if s.Action != "" {
					fmt.Printf("       → %s\n", s.Action)
				}
				if len(s.Source) > 0 {
					fmt.Printf("       source: %s\n", strings.Join(s.Source, ", "))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&apply, "apply", false, "auto-apply allowlisted profile add-skills suggestions")
	cmd.Flags().BoolVar(&applyLocal, "apply-local", false, "also auto-apply Minerva-local skill activate")
	return cmd
}

// --- Template commands ---

func newTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage system prompt templates (builtin + ~/.agents/templates)",
	}
	cmd.AddCommand(
		newTemplateListCmd(),
		newTemplateShowCmd(),
		newTemplateApplyCmd(),
		newTemplateInstallCmd(),
		newTemplateSaveCmd(),
	)
	return cmd
}

func templateDirs() []string {
	return []string{templates.DefaultDir(agentsDir())}
}

func newTemplateListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available templates (disk overrides builtins)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, err := templates.Catalog(templateDirs()...)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(all)
			}
			for _, t := range all {
				src := t.Source
				if src == "" {
					src = "builtin"
				}
				fmt.Printf("%-25s [%s] %s\n", t.Name, src, t.Description)
				if len(t.Skills) > 0 {
					fmt.Printf("  skills: %s\n", strings.Join(t.Skills, ", "))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func newTemplateShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show a template's full prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t := templates.GetFrom(args[0], templateDirs()...)
			if t == nil {
				return fmt.Errorf("template %q not found; available: %s", args[0], strings.Join(templates.NamesFrom(templateDirs()...), ", "))
			}
			fmt.Printf("Name:        %s\n", t.Name)
			fmt.Printf("Source:      %s\n", t.Source)
			if t.Path != "" {
				fmt.Printf("Path:        %s\n", t.Path)
			}
			fmt.Printf("Description: %s\n", t.Description)
			fmt.Printf("Role:        %s\n", t.Role)
			fmt.Printf("Skills:      %s\n", strings.Join(t.Skills, ", "))
			fmt.Printf("\nSystem Prompt:\n%s\n", t.Prompt)
			return nil
		},
	}
	return cmd
}

func newTemplateApplyCmd() *cobra.Command {
	var profileName string
	cmd := &cobra.Command{
		Use:   "apply <template-name>",
		Short: "Apply a template to create or update a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t := templates.GetFrom(args[0], templateDirs()...)
			if t == nil {
				return fmt.Errorf("template %q not found; available: %s", args[0], strings.Join(templates.NamesFrom(templateDirs()...), ", "))
			}

			pmgr := profileManager()
			if err := pmgr.LoadAll(); err != nil {
				return err
			}

			name := profileName
			if name == "" {
				name = t.Name
			}

			existing := pmgr.Get(name)
			if existing != nil {
				if err := pmgr.UpdateSystemPrompt(name, t.Prompt); err != nil {
					return err
				}
				if err := pmgr.UpdateSkills(name, t.Skills); err != nil {
					return err
				}
				fmt.Printf("profile %q updated from template %q\n", name, t.Name)
			} else {
				p := &profile.Profile{
					Name:         name,
					Description:  t.Description,
					Skills:       t.Skills,
					SystemPrompt: t.Prompt,
				}
				if err := pmgr.Create(p); err != nil {
					return err
				}
				fmt.Printf("profile %q created from template %q\n", name, t.Name)
			}

			_ = analyticsStore().Record("template_apply", t.Name, name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&profileName, "profile", "p", "", "profile name (defaults to template name)")
	return cmd
}

func newTemplateInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Copy a builtin template to ~/.agents/templates for editing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := templates.DefaultDir(agentsDir())
			t, err := templates.InstallBuiltin(dir, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("template %q installed to %s\n", t.Name, t.Path)
			return nil
		},
	}
	return cmd
}

func newTemplateSaveCmd() *cobra.Command {
	var description, role, prompt, fromFile string
	var skills []string
	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Save/create a disk template under ~/.agents/templates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := prompt
			if fromFile != "" {
				data, err := os.ReadFile(fromFile)
				if err != nil {
					return err
				}
				body = string(data)
			}
			if strings.TrimSpace(body) == "" {
				return fmt.Errorf("prompt required (--prompt or --from-file)")
			}
			t := templates.Template{
				Name:        args[0],
				Description: description,
				Role:        role,
				Skills:      skills,
				Prompt:      body,
			}
			dir := templates.DefaultDir(agentsDir())
			if err := templates.Save(dir, t); err != nil {
				return err
			}
			fmt.Printf("template %q saved to %s\n", t.Name, filepath.Join(dir, t.Name, "template.yaml"))
			return nil
		},
	}
	cmd.Flags().StringVarP(&description, "description", "d", "", "one-line description")
	cmd.Flags().StringVarP(&role, "role", "r", "", "role label")
	cmd.Flags().StringVar(&prompt, "prompt", "", "system prompt body")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "read prompt from file")
	cmd.Flags().StringSliceVarP(&skills, "skill", "s", nil, "recommended skill names")
	return cmd
}

// --- Library commands ---

func newLibraryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "library",
		Short: "Export, import, and lint the shared agents library",
	}
	cmd.AddCommand(
		newLibraryExportCmd(),
		newLibraryImportCmd(),
		newLibraryLintCmd(),
	)
	return cmd
}

func newLibraryExportCmd() *cobra.Command {
	var note string
	var noTemplates bool
	cmd := &cobra.Command{
		Use:   "export <dest>",
		Short: "Export skills/profiles/templates to a directory or .tar.gz",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := library.Export(library.ExportOptions{
				AgentsDir:        agentsDir(),
				Dest:             args[0],
				IncludeTemplates: !noTemplates,
				Note:             note,
			})
			if err != nil {
				return err
			}
			fmt.Printf("exported %s (%s): skills=%d profiles=%d templates=%d\n",
				res.Path, res.Format, res.Manifest.Skills, res.Manifest.Profiles, res.Manifest.Templates)
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "manifest note")
	cmd.Flags().BoolVar(&noTemplates, "no-templates", false, "omit templates from export")
	return cmd
}

func newLibraryImportCmd() *cobra.Command {
	var force bool
	var noTemplates bool
	cmd := &cobra.Command{
		Use:   "import <source>",
		Short: "Import a library bundle into ~/.agents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := library.Import(library.ImportOptions{
				Source:           args[0],
				AgentsDir:        agentsDir(),
				Force:            force,
				IncludeTemplates: !noTemplates,
			})
			if err != nil {
				return err
			}
			fmt.Printf("imported skills=%d profiles=%d templates=%d skipped=%d\n",
				res.Skills, res.Profiles, res.Templates, res.Skipped)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing skill/profile/template dirs")
	cmd.Flags().BoolVar(&noTemplates, "no-templates", false, "skip templates in the bundle")
	return cmd
}

func newLibraryLintCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Lint skills, profiles, and templates for structural issues",
		Long:  `Checks missing descriptions, broken skill refs, empty prompts, orphan skills, and possible secrets.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := library.Lint(agentsDir())
			if err != nil {
				return err
			}
			if jsonOut {
				if err := printJSON(rep); err != nil {
					return err
				}
			} else {
				fmt.Print(library.FormatHuman(rep))
			}
			if !rep.OK {
				return ExitCode(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

// --- Bridge (local-agent integration) ---

func newBridgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bridge",
		Short: "Generate local-agent integration snippets for a profile",
	}
	cmd.AddCommand(newBridgeShowCmd())
	return cmd
}

func newBridgeShowCmd() *cobra.Command {
	var format, harness, outPath string
	cmd := &cobra.Command{
		Use:   "show <profile>",
		Short: "Print launch + MCP trust docs for a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pmgr := profileManager()
			if err := pmgr.LoadAll(); err != nil {
				return err
			}
			p := pmgr.Get(args[0])
			if p == nil {
				return fmt.Errorf("profile %q not found", args[0])
			}
			fmtFormat := bridge.FormatMarkdown
			switch strings.ToLower(format) {
			case "shell", "sh", "bash":
				fmtFormat = bridge.FormatShell
			case "yaml", "yml":
				fmtFormat = bridge.FormatYAML
			case "md", "markdown", "":
				fmtFormat = bridge.FormatMarkdown
			default:
				return fmt.Errorf("unknown format %q (md|shell|yaml)", format)
			}
			snip, err := bridge.Render(p, bridge.Options{
				AgentsDir:     agentsDir(),
				ProfileName:  p.Name,
				Harness:       harness,
				MinervaBinary: "minerva",
			}, fmtFormat)
			if err != nil {
				return err
			}
			if outPath != "" {
				if err := os.WriteFile(outPath, []byte(snip.Body), 0o644); err != nil {
					return err
				}
				fmt.Printf("wrote %s\n", outPath)
				return nil
			}
			fmt.Print(snip.Body)
			return nil
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "md", "md|shell|yaml")
	cmd.Flags().StringVar(&harness, "harness", "local-agent", "harness name for docs")
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "write to file instead of stdout")
	return cmd
}

// --- Evidence (fcheap conventions) ---

func newEvidenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evidence",
		Short: "Durable evidence helpers via fcheap (tag conventions)",
		Long:  evidence.Docs(),
	}
	cmd.AddCommand(
		newEvidenceSaveCmd(),
		newEvidenceSearchCmd(),
		newEvidenceCloseCmd(),
		newEvidenceDocsCmd(),
	)
	return cmd
}

func newEvidenceSaveCmd() *cobra.Command {
	var name, kind, outcome, ttl string
	var tags []string
	var index bool
	cmd := &cobra.Command{
		Use:   "save <path>",
		Short: "Stash a path with Minerva fcheap tags",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := evidence.Save(cmd.Context(), evidence.SaveRequest{
				Path:    args[0],
				Name:    name,
				Tags:    tags,
				Kind:    kind,
				Outcome: outcome,
				TTL:     ttl,
				Index:   index,
			})
			if err != nil {
				return err
			}
			if res.ID != "" {
				fmt.Printf("stashed id=%s tags=minerva…\n", res.ID)
			} else {
				fmt.Printf("stashed ok\n%s\n", res.Raw)
			}
			_ = analyticsStore().Record("evidence_save", res.ID, kind)
			return nil
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "display name")
	cmd.Flags().StringVarP(&kind, "kind", "k", "eval", "eval|suggest|stack|incident|other")
	cmd.Flags().StringVar(&outcome, "outcome", "", "pass|fail|skip")
	cmd.Flags().StringVar(&ttl, "ttl", "", "ttl e.g. 30d")
	cmd.Flags().StringSliceVarP(&tags, "tag", "t", nil, "extra tags (use skill:name profile:name for suggest attribution)")
	cmd.Flags().BoolVar(&index, "index", true, "index for search after save")
	return cmd
}

func newEvidenceSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search fcheap (defaults to minerva tag query)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := evidence.TagMinerva
			if len(args) == 1 {
				q = args[0]
			}
			out, err := evidence.SearchMinerva(cmd.Context(), q)
			if err != nil {
				return err
			}
			fmt.Print(out)
			if !strings.HasSuffix(out, "\n") {
				fmt.Println()
			}
			return nil
		},
	}
	return cmd
}

func newEvidenceCloseCmd() *cobra.Command {
	var note, kind string
	cmd := &cobra.Command{
		Use:   "close <stash-id>",
		Short: "Mark a fail stash resolved via a close receipt",
		Long: `fcheap has no in-place re-tag. close writes a pass receipt tagged
closes:<id> + outcome:closed. Open fails = outcome:fail without a matching receipt.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := evidence.Close(cmd.Context(), evidence.CloseRequest{
				StashID: args[0],
				Note:    note,
				Kind:    kind,
			})
			if err != nil {
				return err
			}
			fmt.Printf("closed %s", res.ClosedID)
			if res.ReceiptID != "" {
				fmt.Printf(" receipt=%s", res.ReceiptID)
			}
			fmt.Println()
			_ = analyticsStore().Record("evidence_close", res.ClosedID, res.ReceiptID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&note, "note", "n", "", "optional resolution note")
	cmd.Flags().StringVarP(&kind, "kind", "k", "eval", "eval|suggest|stack|incident|other")
	return cmd
}

func newEvidenceDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: "Print Minerva fcheap tag conventions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Print(evidence.Docs())
			return nil
		},
	}
}

// --- MCP command ---

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp <subcommand>",
		Short: "Run Minerva as an MCP server",
	}
	cmd.AddCommand(newMCPServeCmd())
	return cmd
}

func newMCPServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Minerva MCP stdio server",
		Long:  `serve runs Minerva as an MCP stdio server exposing skill management, profile management, stack monitoring, analytics, and self-improvement suggestion tools.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			srv, err := minervamcp.NewServer(agentsDir())
			if err != nil {
				return fmt.Errorf("minerva mcp serve: %w", err)
			}
			if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
				return fmt.Errorf("minerva mcp serve: %w", err)
			}
			return nil
		},
	}
	return cmd
}

// --- Init command ---

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the agents directory structure",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir := agentsDir()
			subdirs := []string{"agents", "skills", "templates", "tasks", "memories"}
			for _, sub := range subdirs {
				path := filepath.Join(dir, sub)
				if err := os.MkdirAll(path, 0o755); err != nil {
					return fmt.Errorf("create %s: %w", sub, err)
				}
			}
			fmt.Printf("initialized agents directory at %s\n", dir)
			return nil
		},
	}
	return cmd
}

// --- Helpers ---

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

func skillManager() *skill.Manager {
	return skill.NewManagerWithState(agentsDir(), filepath.Join(agentsDir(), "skills"))
}

func profileManager() *profile.Manager {
	return profile.NewManager(agentsDir())
}

func analyticsStore() *analytics.Store {
	return analytics.NewStore(agentsDir())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func firstLineCLI(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
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

func profileYAML(p *profile.Profile) (string, error) {
	// Stable, human-oriented projection for diffs (ignore Path).
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\n", p.Name)
	if p.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", p.Description)
	}
	if p.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", p.Model)
	}
	fmt.Fprintf(&b, "skills: [%s]\n", strings.Join(p.Skills, ", "))
	fmt.Fprintf(&b, "mcp_servers: [%s]\n", strings.Join(p.MCPServers, ", "))
	if len(p.UseCases) > 0 {
		fmt.Fprintf(&b, "use_cases: [%s]\n", strings.Join(p.UseCases, ", "))
	}
	fmt.Fprintf(&b, "system_prompt: |\n")
	for _, line := range strings.Split(p.SystemPrompt, "\n") {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	return b.String(), nil
}

// resolveContentArg returns content from args[idx] or --from-file.
func resolveContentArg(args []string, idx int, fromFile string) (string, error) {
	if fromFile != "" {
		data, err := os.ReadFile(fromFile)
		if err != nil {
			return "", fmt.Errorf("read --from-file: %w", err)
		}
		return string(data), nil
	}
	if idx < len(args) {
		return args[idx], nil
	}
	return "", nil
}

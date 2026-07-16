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

	"github.com/spf13/cobra"

	"github.com/abdul-hamid-achik/minerva/internal/analytics"
	"github.com/abdul-hamid-achik/minerva/internal/evidence"
	"github.com/abdul-hamid-achik/minerva/internal/integration"
	minervamcp "github.com/abdul-hamid-achik/minerva/internal/mcp"
	"github.com/abdul-hamid-achik/minerva/internal/monitor"
	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/suggest"
	"github.com/abdul-hamid-achik/minerva/internal/templates"
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
		newAnalyticsCmd(),
		newSuggestCmd(),
		newTemplateCmd(),
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
	cmd := &cobra.Command{
		Use:   "compare <name-a> <name-b>",
		Short: "Compare two skills side by side",
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
			fmt.Printf("=== %s ===\n%s\n\n=== %s ===\n%s\n", args[0], contentA, args[1], contentB)
			return nil
		},
	}
	return cmd
}

func newSkillCreateCmd() *cobra.Command {
	var description string
	cmd := &cobra.Command{
		Use:   "create <name> <content>",
		Short: "Create a new skill",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			skillsDir := filepath.Join(agentsDir(), "skills")
			if err := mgr.Create(skillsDir, args[0], description, args[1]); err != nil {
				return err
			}
			_ = analyticsStore().Record("skill_create", args[0], description)
			fmt.Printf("skill %q created\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&description, "description", "d", "", "one-line description")
	return cmd
}

func newSkillActivateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate <name>",
		Short: "Activate a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if err := mgr.Activate(args[0]); err != nil {
				return err
			}
			_ = analyticsStore().Record("skill_activate", args[0], "")
			fmt.Printf("skill %q activated\n", args[0])
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
				return printJSON(mgr.All())
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
	cmd := &cobra.Command{
		Use:   "compare <name-a> <name-b>",
		Short: "Compare two profiles side by side",
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
		},
	}
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
		Short: "Update a profile's skills",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := profileManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			skills := strings.Split(args[1], ",")
			for i, s := range skills {
				skills[i] = strings.TrimSpace(s)
			}
			if err := mgr.UpdateSkills(args[0], skills); err != nil {
				return err
			}
			_ = analyticsStore().Record("profile_update_skills", args[0], strings.Join(skills, ","))
			fmt.Printf("skills updated for profile %q\n", args[0])
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
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check intelligence stack presence (tiered; correct binaries)",
		Long:  `Probes PATH for stack tools using real binary names (glyph, cairn, tvault). Core tools missing → unhealthy; optional missing → degraded. Domain readiness is under stack deep.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonOut {
				data, err := monitor.CheckStackJSON()
				if err != nil {
					return err
				}
				fmt.Println(string(data))
				return nil
			}
			status := monitor.CheckStack()
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
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func newStackDeepCmd() *cobra.Command {
	var workspace string
	var jsonOut, stash bool
	cmd := &cobra.Command{
		Use:   "deep [workspace]",
		Short: "Deep stack probe (bob/cortex/mcphub + readiness doctors)",
		Long: `Composes sibling CLI contracts: bob check/context, cortex doctor, mcphub stats,
plus optional codemap/vecgrep/fcheap/tvault/monitor readiness probes.

Sets retrieval_ready only when both codemap and vecgrep are domain-ready.
--stash writes the JSON report to fcheap with minerva-stack tags (TTL 30d).`,
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
				return printJSON(status)
			}
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
				fmt.Printf("cortex:  version=%s source=%s ready=%v\n", status.Cortex.Version, status.Cortex.Source, status.Cortex.Ready)
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
			return nil
		},
	}
	cmd.Flags().StringVarP(&workspace, "workspace", "C", "", "workspace directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&stash, "stash", false, "save report to fcheap (minerva-stack tags, TTL 30d)")
	return cmd
}

// --- Analytics command ---

func newAnalyticsCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "analytics",
		Short: "View usage analytics",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := analyticsStore()
			_ = store.Load()
			summary := store.Summarize()
			if jsonOut {
				return printJSON(summary)
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
	var jsonOut, apply bool
	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "Get library and stack improvement suggestions",
		Long: `Analyze skills, profiles, stack presence, analytics, and workspace type.

Activation suggestions update Minerva's local state only
(~/.agents/.minerva-skills.json). They do not inject skills into a live
local-agent session. Prefer profile skill lists for durable harness behavior.

--apply only runs allowlisted "minerva skill activate <name>" actions.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			pmgr := profileManager()
			if err := pmgr.LoadAll(); err != nil {
				return err
			}
			store := analyticsStore()
			_ = store.Load()

			ws, _ := os.Getwd()
			engine := suggest.NewEngine(mgr, pmgr, store, ws)
			engine.IncludeReadiness = true
			engine.IncludeEvidence = true
			suggestions := engine.Analyze()

			if apply {
				applied, skipped, err := suggest.ApplyAuto(mgr, suggestions)
				if err != nil {
					return err
				}
				for _, name := range applied {
					fmt.Printf("activated: %s\n", name)
				}
				for _, s := range skipped {
					fmt.Printf("skipped: %s\n", s)
				}
				_ = store.Record("suggestion_applied", fmt.Sprintf("%d", len(applied)), strings.Join(applied, ","))
				fmt.Printf("\n%d skills activated, %d skipped\n", len(applied), len(skipped))
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
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&apply, "apply", false, "auto-apply allowlisted activate suggestions")
	return cmd
}

// --- Template commands ---

func newTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage system prompt templates",
	}
	cmd.AddCommand(
		newTemplateListCmd(),
		newTemplateShowCmd(),
		newTemplateApplyCmd(),
	)
	return cmd
}

func newTemplateListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available templates",
		RunE: func(cmd *cobra.Command, _ []string) error {
			all := templates.All()
			if jsonOut {
				return printJSON(all)
			}
			for _, t := range all {
				fmt.Printf("%-25s %s\n", t.Name, t.Description)
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
			t := templates.Get(args[0])
			if t == nil {
				return fmt.Errorf("template %q not found; available: %s", args[0], strings.Join(templates.Names(), ", "))
			}
			fmt.Printf("Name:        %s\n", t.Name)
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
			t := templates.Get(args[0])
			if t == nil {
				return fmt.Errorf("template %q not found; available: %s", args[0], strings.Join(templates.Names(), ", "))
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
				// Update existing profile
				if err := pmgr.UpdateSystemPrompt(name, t.Prompt); err != nil {
					return err
				}
				if err := pmgr.UpdateSkills(name, t.Skills); err != nil {
					return err
				}
				fmt.Printf("profile %q updated from template %q\n", name, t.Name)
			} else {
				// Create new profile
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
			subdirs := []string{"agents", "skills", "tasks", "memories"}
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

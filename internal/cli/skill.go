package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/abdul-hamid-achik/minerva/internal/library"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/sync"
	"github.com/abdul-hamid-achik/minerva/internal/textdiff"
)

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "skill", Short: "Manage the canonical skill library"}
	cmd.AddCommand(
		newSkillListCmd(),
		newSkillShowCmd(),
		newSkillCompareCmd(),
		newSkillCreateCmd(),
		newSkillUpdateCmd(),
		newSkillDeleteCmd(),
		newSkillResolveCmd(),
		newSkillLintCmd(),
		newSkillInstallCmd(),
		newSkillSyncCmd(),
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
				fmt.Printf("  %s", s.Name)
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
	return &cobra.Command{
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
			if err := mgr.Create(filepath.Join(agentsDir(), "skills"), args[0], description, content); err != nil {
				return err
			}
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

func newSkillDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			if err := mgr.Delete(filepath.Join(agentsDir(), "skills"), args[0]); err != nil {
				return err
			}
			fmt.Printf("skill %q deleted\n", args[0])
			return nil
		},
	}
}

func newSkillResolveCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "resolve <intent>",
		Short: "Rank skills for a natural-language intent",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := skillManager()
			if err := mgr.LoadAll(); err != nil {
				return err
			}
			res := skill.Resolve(strings.Join(args, " "), mgr.All())
			if jsonOut {
				return printJSON(res)
			}
			if len(res.Hits) == 0 {
				fmt.Println("no matching skills")
				return nil
			}
			for i, hit := range res.Hits {
				fmt.Printf("%d. %s  score=%d  %s\n", i+1, hit.Name, hit.Score, hit.Reason)
				fmt.Printf("   → %s\n", hit.Action)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func newSkillLintCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Lint skills for structural issues",
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

func newSkillInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <owner/repo[/path]>",
		Short: "Clone a GitHub skill into ~/.agents/skills and update .skill-lock.json",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := sync.Install(agentsDir(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("skill %q installed\n", name)
			return nil
		},
	}
}

func newSkillSyncCmd() *cobra.Command {
	var to string
	var dryRun, jsonOut bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Link or copy canonical skills into harness skill dirs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			targets := splitCSV(to)
			acts, err := sync.Sync(sync.SyncOptions{Env: env(), To: targets, DryRun: dryRun})
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(acts)
			}
			if len(acts) == 0 {
				fmt.Println("nothing to sync")
				return nil
			}
			for _, a := range acts {
				state := "planned"
				if a.Done {
					state = "done"
				}
				fmt.Printf("  [%s] %s %s %s → %s\n", state, a.Harness, a.Method, a.Skill, a.To)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "comma-separated harness ids (default: all writable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print actions without writing")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

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

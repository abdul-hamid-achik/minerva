package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
	"github.com/abdul-hamid-achik/minerva/internal/sync"
)

func newHarnessCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "harness", Short: "List harnesses and check skill-dir drift"}
	cmd.AddCommand(newHarnessListCmd(), newHarnessDoctorCmd())
	return cmd
}

func newHarnessListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List known harnesses and whether their data is present",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat := harness.Catalog(env())
			if jsonOut {
				return printJSON(cat)
			}
			for _, h := range cat {
				mark := " "
				if h.Present {
					mark = "*"
				}
				fmt.Printf(" [%s] %-10s  skills=%s\n", mark, h.ID, h.SkillsDir)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func newHarnessDoctorCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Compare ~/.agents/skills with each harness skills dir",
		RunE: func(cmd *cobra.Command, _ []string) error {
			findings, err := sync.Doctor(env())
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(findings)
			}
			if len(findings) == 0 {
				fmt.Println("no drift")
				return nil
			}
			for _, f := range findings {
				fmt.Printf("  [%s] %s %s — %s\n", f.Kind, f.Harness, f.Skill, f.Message)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

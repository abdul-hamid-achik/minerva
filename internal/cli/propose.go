package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/abdul-hamid-achik/minerva/internal/propose"
	"github.com/abdul-hamid-achik/minerva/internal/session"
	"github.com/abdul-hamid-achik/minerva/internal/signal"
)

func newProposeCmd() *cobra.Command {
	var jsonOut bool
	var harnessID, workspace, since string
	var limit int
	run := func(cmd *cobra.Command, _ []string) error {
		dur, err := parseSince(since)
		if err != nil {
			return err
		}
		if limit <= 0 {
			limit = 40
		}
		sessions, err := session.LoadFiltered(env(), session.Filter{
			Harness: harnessID, Workspace: workspace, Since: dur, Limit: limit,
		})
		if err != nil {
			return err
		}
		for i := range sessions {
			session.RedactSession(&sessions[i])
		}
		mgr := catalogForAnalysis()
		sigs := signal.Extract(sessions, mgr.All())
		proposals := propose.FromSignals(sigs, mgr.All())
		if err := propose.Save(agentsDir(), proposals); err != nil {
			return err
		}
		if jsonOut {
			return printJSON(map[string]any{"sessions": len(sessions), "proposals": proposals})
		}
		if len(proposals) == 0 {
			fmt.Println("no proposals")
			return nil
		}
		for _, p := range proposals {
			fmt.Printf("  [%s] %s  %s\n", p.Kind, p.ID, p.Title)
			fmt.Printf("       %s\n", p.Action)
		}
		return nil
	}

	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Propose skills from session signals",
		RunE:  run,
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().StringVar(&harnessID, "harness", "", "filter by harness id")
	cmd.Flags().StringVar(&workspace, "workspace", "", "only sessions in this workspace (path, or its last segment)")
	cmd.Flags().StringVar(&since, "since", "30d", "age window (24h, 7d)")
	cmd.Flags().IntVar(&limit, "limit", 40, "max sessions (counted after --workspace)")

	apply := &cobra.Command{
		Use:   "apply <proposal-id>",
		Short: "Write a new_skill or update_skill proposal to disk",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := propose.Find(agentsDir(), args[0])
			if err != nil {
				return err
			}
			path, err := propose.Apply(agentsDir(), *p)
			if err != nil {
				return err
			}
			fmt.Printf("applied %s → %s\n", p.ID, path)
			return nil
		},
	}
	cmd.AddCommand(apply)
	return cmd
}

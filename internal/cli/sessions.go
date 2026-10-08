package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/abdul-hamid-achik/minerva/internal/session"
)

func newSessionsCmd() *cobra.Command {
	var jsonOut bool
	var harnessID, workspace, since string
	var limit int
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List harness conversation files",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dur, err := parseSince(since)
			if err != nil {
				return err
			}
			list, err := session.List(env(), session.Filter{
				Harness: harnessID, Workspace: workspace, Since: dur, Limit: limit,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(list)
			}
			if len(list) == 0 {
				fmt.Println("no sessions found")
				return nil
			}
			for _, s := range list {
				fmt.Printf("  %-10s  %s  %s\n", s.Harness, s.ID, s.MTime.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().StringVar(&harnessID, "harness", "", "filter by harness id")
	cmd.Flags().StringVar(&workspace, "workspace", "", "only sessions in this workspace (path, or its last segment)")
	cmd.Flags().StringVar(&since, "since", "", "age window (24h, 7d)")
	cmd.Flags().IntVar(&limit, "limit", 30, "max sessions")
	return cmd
}

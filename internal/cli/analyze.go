package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/abdul-hamid-achik/minerva/internal/session"
	"github.com/abdul-hamid-achik/minerva/internal/signal"
)

func newAnalyzeCmd() *cobra.Command {
	var jsonOut, last bool
	var harnessID, workspace, since, sessionID string
	var limit int
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Extract skill signals from harness sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dur, err := parseSince(since)
			if err != nil {
				return err
			}
			filter := session.Filter{Harness: harnessID, Workspace: workspace, Since: dur, Limit: limit, SessionID: sessionID}
			if last {
				filter.Limit = 1
			}
			if filter.Limit == 0 {
				filter.Limit = 20
			}
			sessions, err := session.LoadFiltered(env(), filter)
			if err != nil {
				return err
			}
			for i := range sessions {
				session.RedactSession(&sessions[i])
			}
			mgr := catalogForAnalysis()
			sigs := signal.Extract(sessions, mgr.All())
			payload := map[string]any{"sessions": len(sessions), "signals": sigs}
			if jsonOut {
				return printJSON(payload)
			}
			fmt.Printf("analyzed %d session(s), %d signal(s)\n", len(sessions), len(sigs))
			for _, s := range sigs {
				fmt.Printf("  [%s] %s (weight=%d)\n", s.Kind, s.Message, s.Weight)
				if len(s.Evidence) > 0 {
					fmt.Printf("       %s %s\n", s.Evidence[0].Harness, s.Evidence[0].SessionID)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&last, "last", false, "analyze only the newest session")
	cmd.Flags().StringVar(&harnessID, "harness", "", "filter by harness id")
	cmd.Flags().StringVar(&workspace, "workspace", "", "only sessions in this workspace (path, or its last segment)")
	cmd.Flags().StringVar(&since, "since", "", "age window (24h, 7d)")
	cmd.Flags().StringVar(&sessionID, "session", "", "session id prefix (or a Codex rollout uuid prefix)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max sessions (counted after --workspace)")
	return cmd
}

package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	minervamcp "github.com/abdul-hamid-achik/minerva/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Run Minerva as an MCP server"}
	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Start the Minerva MCP stdio server",
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
	})
	return cmd
}

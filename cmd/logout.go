package cmd

import (
	"fmt"

	"github.com/lisiting01/aitoll-cli/internal/auth"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials",
	Long:  "Remove the locally stored authentication token.",
	RunE:  runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(cmd *cobra.Command, args []string) error {
	store := auth.NewTokenStore()

	creds, err := store.Load()
	if err != nil {
		return fmt.Errorf("failed to read credentials: %w", err)
	}

	if creds == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "Not logged in.")
		return nil
	}

	if err := store.Delete(); err != nil {
		return fmt.Errorf("failed to remove credentials: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Logged out successfully.")
	return nil
}

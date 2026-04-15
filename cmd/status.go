package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/auth"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	Long:  "Display the current authentication status and user information.",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	store := auth.NewTokenStore()

	creds, err := store.Load()
	if err != nil {
		return fmt.Errorf("failed to read credentials: %w", err)
	}

	if creds == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "Not logged in. Run 'aitoll login' to authenticate.")
		return nil
	}

	claims, err := auth.DecodeTokenClaims(creds.Token)
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Stored token is invalid. Run 'aitoll login' to re-authenticate.\n")
		return nil
	}

	// Check expiry
	expiry := time.Unix(claims.Exp, 0)
	now := time.Now()
	if now.After(expiry) {
		fmt.Fprintf(cmd.OutOrStdout(), "Session expired (expired at %s). Run 'aitoll login' to re-authenticate.\n",
			expiry.Format("2006-01-02 15:04:05"))
		return nil
	}

	remaining := time.Until(expiry)
	fmt.Fprintf(cmd.OutOrStdout(), "Logged in to %s\n\n", creds.BaseURL)
	fmt.Fprintf(cmd.OutOrStdout(), "  Username:  %s\n", claims.Username)
	fmt.Fprintf(cmd.OutOrStdout(), "  Email:     %s\n", claims.Email)
	fmt.Fprintf(cmd.OutOrStdout(), "  Role:      %s\n", claims.Role)
	if claims.UserType != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Type:      %s\n", claims.UserType)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  Expires:   %s (%s)\n",
		expiry.Format("2006-01-02 15:04:05"),
		formatDuration(remaining))

	return nil
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 && days == 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	if len(parts) == 0 {
		return "< 1m"
	}
	return strings.Join(parts, " ")
}

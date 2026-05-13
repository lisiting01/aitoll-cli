package cmd

import (
	"fmt"

	"github.com/lisiting01/aitoll-cli/internal/updater"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install the latest version of aitoll",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(cmd.OutOrStdout(), "Checking for updates...")

	info, err := updater.CheckLatest()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	if !updater.IsNewer(appVersion, info.TagName) {
		fmt.Fprintf(cmd.OutOrStdout(), "Already up to date (%s).\n", appVersion)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "New version available: %s (current: %s)\n", info.TagName, appVersion)

	asset, err := updater.FindAsset(info)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Downloading %s...\n", asset.Name)
	if err := updater.DownloadAndReplace(asset); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Updated to %s. Restart aitoll to use the new version.\n", info.TagName)
	return nil
}

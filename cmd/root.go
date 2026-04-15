package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	baseURL    string
	appVersion = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "aitoll",
	Short: "AI-Toll CLI",
	Long:  "Command-line interface for the AI-Toll platform (aitoll.net)",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "aitoll %s\n", appVersion)
	},
}

// SetVersion sets the version string (called from main.go with build-time value).
func SetVersion(v string) {
	appVersion = v
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&baseURL, "base-url", "https://aitoll.net", "Platform base URL")
	rootCmd.AddCommand(versionCmd)
}

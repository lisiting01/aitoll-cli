package cmd

import (
	"fmt"
	"strings"

	"github.com/lisiting01/aitoll-cli/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage aitoll CLI configuration",
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd, configSetCmd)
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := strings.ToLower(args[0])
	cfg, err := config.LoadAppConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	switch key {
	case "auto-update":
		fmt.Fprintf(cmd.OutOrStdout(), "auto-update = %v\n", cfg.AutoUpdate)
	default:
		return fmt.Errorf("unknown config key %q (available: auto-update)", key)
	}
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := strings.ToLower(args[0])
	val := strings.ToLower(args[1])

	cfg, err := config.LoadAppConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	switch key {
	case "auto-update":
		switch val {
		case "true", "1", "yes", "on":
			cfg.AutoUpdate = true
		case "false", "0", "no", "off":
			cfg.AutoUpdate = false
		default:
			return fmt.Errorf("invalid value %q for auto-update (use true/false)", val)
		}
	default:
		return fmt.Errorf("unknown config key %q (available: auto-update)", key)
	}

	if err := config.SaveAppConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, val)
	return nil
}

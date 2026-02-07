package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/buddyh/alexa-cli/internal/api"
	"github.com/buddyh/alexa-cli/internal/config"
	"github.com/buddyh/alexa-cli/internal/output"
	"github.com/spf13/cobra"
)

type rootFlags struct {
	asJSON  bool
	verbose bool
}

func execute(args []string) error {
	flags := &rootFlags{}

	rootCmd := &cobra.Command{
		Use:   "alexacli",
		Short: "CLI for controlling Alexa devices",
		Long: `A command-line interface for controlling Amazon Alexa devices.

Supports TTS announcements, smart home control, routine execution,
and sending arbitrary voice commands.

Get started by running 'alexacli auth' to configure your refresh token.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().BoolVar(&flags.asJSON, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().BoolVarP(&flags.verbose, "verbose", "v", false, "Enable verbose debug output")

	// Add commands
	rootCmd.AddCommand(newAuthCmd(flags))
	rootCmd.AddCommand(newDevicesCmd(flags))
	rootCmd.AddCommand(newSpeakCmd(flags))
	rootCmd.AddCommand(newCommandCmd(flags))
	rootCmd.AddCommand(newAskCmd(flags))
	rootCmd.AddCommand(newAskPlusCmd(flags))
	rootCmd.AddCommand(newConversationsCmd(flags))
	rootCmd.AddCommand(newFragmentsCmd(flags))
	rootCmd.AddCommand(newHistoryCmd(flags))
	rootCmd.AddCommand(newPlayCmd(flags))
	rootCmd.AddCommand(newRoutineCmd(flags))
	rootCmd.AddCommand(newSmartHomeCmd(flags))

	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// getClient creates an authenticated Alexa API client
func getClient() (*api.Client, error) {
	return getClientWithFlags(nil)
}

// getClientWithFlags creates an authenticated Alexa API client with optional flags
func getClientWithFlags(flags *rootFlags) (*api.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	client, err := api.NewClient(cfg.RefreshToken, cfg.AmazonDomain)
	if err != nil {
		return nil, err
	}

	if flags != nil && flags.verbose {
		client.SetVerbose(true)
	}

	return client, nil
}

// getFormatter creates an output formatter
func getFormatter(flags *rootFlags) *output.Formatter {
	return output.NewFormatter(os.Stdout, flags.asJSON)
}

// findDevice finds a device by name or serial
func findDevice(client *api.Client, nameOrSerial string) (*api.Device, error) {
	devices, err := client.GetDevices()
	if err != nil {
		return nil, err
	}

	// Try exact match first
	for i, d := range devices {
		if d.SerialNumber == nameOrSerial || d.AccountName == nameOrSerial {
			return &devices[i], nil
		}
	}

	// Try case-insensitive partial match
	nameLower := strings.ToLower(nameOrSerial)
	for i, d := range devices {
		if strings.Contains(strings.ToLower(d.AccountName), nameLower) {
			return &devices[i], nil
		}
	}

	return nil, fmt.Errorf("device '%s' not found", nameOrSerial)
}

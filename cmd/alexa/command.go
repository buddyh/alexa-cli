package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newCommandCmd(flags *rootFlags) *cobra.Command {
	var device string

	cmd := &cobra.Command{
		Use:   "command <text>",
		Short: "Send a voice command to Alexa",
		Long: `Send an arbitrary voice command to Alexa, as if you spoke it.

This is equivalent to saying "Alexa, <text>" to the device.

Examples:
  alexacli command "turn off all lights" -d "Kitchen Echo"
  alexacli command "play jazz" -d Office
  alexacli command "what's the weather" -d Bedroom
  alexacli command "set a timer for 5 minutes" -d Kitchen`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)

			client, err := getClientWithFlags(flags)
			if err != nil {
				return err
			}

			if device == "" {
				return fmt.Errorf("device is required (use -d)")
			}

			dev, err := findDevice(client, device)
			if err != nil {
				return err
			}

			text := strings.Join(args, " ")

			if err := client.SequenceCommand(dev, fmt.Sprintf("textcommand:'%s'", text)); err != nil {
				return err
			}

			return out.Success(fmt.Sprintf("Command sent to %s: %s", dev.AccountName, text))
		},
	}

	cmd.Flags().StringVarP(&device, "device", "d", "", "Device name or serial (required)")
	cmd.MarkFlagRequired("device")

	return cmd
}

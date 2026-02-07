package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newSpeakCmd(flags *rootFlags) *cobra.Command {
	var device string
	var announce bool

	cmd := &cobra.Command{
		Use:   "speak <text>",
		Short: "Make Alexa speak text",
		Long: `Make an Alexa device speak the provided text using text-to-speech.

Use --announce to broadcast to all devices.

Examples:
  alexacli speak "Hello world" -d "Kitchen Echo"
  alexacli speak "Dinner is ready" --announce
  alexacli speak "The build completed successfully" -d Office`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)

			client, err := getClientWithFlags(flags)
			if err != nil {
				return err
			}

			text := strings.Join(args, " ")

			if announce {
				// Announcement goes to all devices
				devices, err := client.GetDevices()
				if err != nil {
					return err
				}

				if len(devices) == 0 {
					return fmt.Errorf("no devices found")
				}

				// Use first device to trigger announcement
				if err := client.SequenceCommand(&devices[0], fmt.Sprintf("announcement:'%s'", text)); err != nil {
					return err
				}

				return out.Success(fmt.Sprintf("Announced: %s", text))
			}

			// Send to specific device
			if device == "" {
				return fmt.Errorf("device is required (use -d or --announce)")
			}

			dev, err := findDevice(client, device)
			if err != nil {
				return err
			}

			if err := client.SequenceCommand(dev, fmt.Sprintf("speak:'%s'", text)); err != nil {
				return err
			}

			return out.Success(fmt.Sprintf("Spoke on %s: %s", dev.AccountName, text))
		},
	}

	cmd.Flags().StringVarP(&device, "device", "d", "", "Device name or serial")
	cmd.Flags().BoolVarP(&announce, "announce", "a", false, "Announce to all devices")

	return cmd
}

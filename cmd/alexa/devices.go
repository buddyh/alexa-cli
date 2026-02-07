package main

import (
	"github.com/buddyh/alexa-cli/internal/output"
	"github.com/spf13/cobra"
)

func newDevicesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "List Alexa devices",
		Long:  `List all Alexa devices registered to your account.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)

			client, err := getClientWithFlags(flags)
			if err != nil {
				return err
			}

			devices, err := client.GetDevices()
			if err != nil {
				return err
			}

			// Convert to output format
			outputDevices := make([]output.Device, 0, len(devices))
			for _, d := range devices {
				// Filter to only Echo-like devices
				if d.DeviceFamily == "ECHO" || d.DeviceFamily == "KNIGHT" || d.DeviceFamily == "ROOK" {
					outputDevices = append(outputDevices, output.Device{
						Name:   d.AccountName,
						Serial: d.SerialNumber,
						Type:   d.DeviceType,
						Family: d.DeviceFamily,
						Online: d.Online,
					})
				}
			}

			return out.Devices(outputDevices)
		},
	}

	return cmd
}

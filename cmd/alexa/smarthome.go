package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/buddyh/alexa-cli/internal/api"
	"github.com/spf13/cobra"
)

func newSmartHomeCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "smarthome",
		Short:   "Control smart home devices",
		Long:    `List and control smart home devices connected to Alexa.`,
		Aliases: []string{"sh", "home"},
	}

	cmd.AddCommand(newSmartHomeListCmd(flags))
	cmd.AddCommand(newSmartHomeOnCmd(flags))
	cmd.AddCommand(newSmartHomeOffCmd(flags))
	cmd.AddCommand(newSmartHomeBrightnessCmd(flags))

	return cmd
}

func newSmartHomeListCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List smart home devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)

			client, err := getClient()
			if err != nil {
				return err
			}

			devices, err := client.GetSmartHomeDevices()
			if err != nil {
				return err
			}

			if flags.asJSON {
				return out.Data(devices)
			}

			if len(devices) == 0 {
				return out.Success("No smart home devices found")
			}

			for _, d := range devices {
				status := ""
				if !d.Reachable {
					status = " (unreachable)"
				}
				fmt.Printf("%-30s %s%s\n", d.Name, d.Type, status)
			}
			return nil
		},
	}
}

func newSmartHomeOnCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "on <device-name>",
		Short: "Turn on a device",
		Long: `Turn on a smart home device.

Examples:
  alexacli smarthome on "Kitchen Light"
  alexacli sh on "Living Room Lamp"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)

			client, err := getClient()
			if err != nil {
				return err
			}

			deviceName := args[0]
			device, err := findSmartDevice(client, deviceName)
			if err != nil {
				return err
			}

			if err := client.ControlSmartHome(device.EntityID, "on", nil); err != nil {
				return err
			}

			return out.Success(fmt.Sprintf("Turned on: %s", device.Name))
		},
	}
}

func newSmartHomeOffCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "off <device-name>",
		Short: "Turn off a device",
		Long: `Turn off a smart home device.

Examples:
  alexacli smarthome off "Kitchen Light"
  alexacli sh off "All Lights"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)

			client, err := getClient()
			if err != nil {
				return err
			}

			deviceName := args[0]
			device, err := findSmartDevice(client, deviceName)
			if err != nil {
				return err
			}

			if err := client.ControlSmartHome(device.EntityID, "off", nil); err != nil {
				return err
			}

			return out.Success(fmt.Sprintf("Turned off: %s", device.Name))
		},
	}
}

func newSmartHomeBrightnessCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "brightness <device-name> <level>",
		Short: "Set device brightness",
		Long: `Set the brightness of a smart home device.

Level is 0-100.

Examples:
  alexacli smarthome brightness "Kitchen Light" 50
  alexacli sh brightness "Bedroom Lamp" 75`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)

			client, err := getClient()
			if err != nil {
				return err
			}

			deviceName := args[0]
			level, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid brightness level: %w", err)
			}

			if level < 0 || level > 100 {
				return fmt.Errorf("brightness must be 0-100")
			}

			device, err := findSmartDevice(client, deviceName)
			if err != nil {
				return err
			}

			if err := client.ControlSmartHome(device.EntityID, "brightness", level); err != nil {
				return err
			}

			return out.Success(fmt.Sprintf("Set %s brightness to %d%%", device.Name, level))
		},
	}
}

// findSmartDevice finds a smart home device by name
func findSmartDevice(client *api.Client, name string) (*api.SmartHomeDevice, error) {
	devices, err := client.GetSmartHomeDevices()
	if err != nil {
		return nil, err
	}

	nameLower := strings.ToLower(name)
	for i, d := range devices {
		if strings.ToLower(d.Name) == nameLower {
			return &devices[i], nil
		}
	}

	// Try partial match
	for i, d := range devices {
		if strings.Contains(strings.ToLower(d.Name), nameLower) {
			return &devices[i], nil
		}
	}

	return nil, fmt.Errorf("smart home device '%s' not found", name)
}

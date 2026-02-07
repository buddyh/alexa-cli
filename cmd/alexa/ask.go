package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newAskCmd(flags *rootFlags) *cobra.Command {
	var device string
	var timeout int

	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Ask Alexa a question and get the response",
		Long: `Send a question to Alexa and wait for the response.

This command sends your question as a voice command, then polls
Alexa's activity history to retrieve what Alexa said in response.

This is the killer feature for AI/agent integration - you can now
get actual answers back from Alexa, not just send one-way commands.

Examples:
  alexacli ask "what's the weather" -d Kitchen
  alexacli ask "is the front door locked" -d Kitchen
  alexacli ask "what's on my calendar today" -d Office
  alexacli ask "what time is it" -d Bedroom`,
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

			question := strings.Join(args, " ")
			timeoutDuration := time.Duration(timeout) * time.Second

			response, err := client.Ask(dev, question, timeoutDuration)
			if err != nil {
				return err
			}

			if flags.asJSON {
				return out.Data(map[string]string{
					"question": question,
					"response": response,
					"device":   dev.AccountName,
				})
			}

			fmt.Println(response)
			return nil
		},
	}

	cmd.Flags().StringVarP(&device, "device", "d", "", "Device name or serial (required)")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 10, "Timeout in seconds to wait for response")
	cmd.MarkFlagRequired("device")

	return cmd
}

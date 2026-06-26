package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/buddyh/alexa-cli/internal/api"
	"github.com/buddyh/alexa-cli/internal/config"
	"github.com/buddyh/alexa-cli/internal/watch"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := execute(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func execute(args []string) error {
	root := &cobra.Command{
		Use:           "alexa-poller",
		Short:         "Listen for Alexa activation phrases and route them to automations",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.AddCommand(newRouteCmd())
	root.AddCommand(newRunCmd())
	root.SetArgs(args)
	return root.Execute()
}

func newRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Manage activation phrase routes",
	}
	cmd.AddCommand(newRouteAddCmd())
	cmd.AddCommand(newRouteListCmd())
	cmd.AddCommand(newRouteRemoveCmd())
	return cmd
}

func newRouteAddCmd() *cobra.Command {
	var device string
	var source string
	var destination string
	var ack string
	var ignore []string
	var match string

	cmd := &cobra.Command{
		Use:   "add <phrase>",
		Short: "Add an activation phrase route",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := watch.LoadConfig()
			if err != nil {
				return err
			}

			action, err := parseDestination(destination)
			if err != nil {
				return err
			}

			route := watch.Route{
				Phrase: args[0],
				Device: device,
				Source: watch.Source(source),
				Match:  watch.Match(match),
				Ignore: ignore,
				Action: action,
			}
			if ack != "" {
				route.Ack = &watch.Ack{Text: ack}
			}
			if err := route.Normalize(); err != nil {
				return err
			}

			cfg.Routes = append(cfg.Routes, route)
			if err := watch.SaveConfig(cfg); err != nil {
				return err
			}

			fmt.Printf("Added route %s for phrase %q\n", route.ID, route.Phrase)
			return nil
		},
	}

	cmd.Flags().StringVar(&device, "device", "", "Limit matches to one Alexa device")
	cmd.Flags().StringVar(&source, "source", string(watch.SourceAuto), "Source backend: auto, conversation, history")
	cmd.Flags().StringVar(&destination, "to", "openclaw:main", "Route destination, e.g. openclaw:main, exec:/path/to/script '{{text}}', stdout")
	cmd.Flags().StringVar(&ack, "ack", "", "Optional Alexa acknowledgement text")
	cmd.Flags().StringSliceVar(&ignore, "ignore", nil, "Additional ignore phrases")
	cmd.Flags().StringVar(&match, "match", string(watch.MatchPrefix), "Phrase match mode: prefix or contains")
	return cmd
}

func newRouteListCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured routes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := watch.LoadConfig()
			if err != nil {
				return err
			}

			if asJSON {
				data, err := json.MarshalIndent(cfg.Routes, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(data))
				return nil
			}

			if len(cfg.Routes) == 0 {
				fmt.Println("No routes configured.")
				return nil
			}

			for _, route := range cfg.Routes {
				dest := string(route.Action.Type)
				switch route.Action.Type {
				case watch.ActionOpenClaw:
					dest += ":" + route.Action.Agent
				case watch.ActionExec:
					dest += ":" + route.Action.Command
				}
				fmt.Printf("%s  phrase=%q device=%q source=%s dest=%s\n",
					route.ID, route.Phrase, route.Device, route.Source, dest)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output routes as JSON")
	return cmd
}

func newRouteRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <route-id>",
		Short: "Remove a configured route",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := watch.LoadConfig()
			if err != nil {
				return err
			}

			routeID := args[0]
			filtered := make([]watch.Route, 0, len(cfg.Routes))
			removed := false
			for _, route := range cfg.Routes {
				if route.ID == routeID {
					removed = true
					continue
				}
				filtered = append(filtered, route)
			}
			if !removed {
				return fmt.Errorf("route %q not found", routeID)
			}

			cfg.Routes = filtered
			if err := watch.SaveConfig(cfg); err != nil {
				return err
			}

			fmt.Printf("Removed route %s\n", routeID)
			return nil
		},
	}
	return cmd
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Alexa poller",
		RunE: func(cmd *cobra.Command, args []string) error {
			pollerCfg, err := watch.LoadConfig()
			if err != nil {
				return err
			}
			if len(pollerCfg.Routes) == 0 {
				return fmt.Errorf("no routes configured. Add one with 'alexa-poller route add'")
			}

			state, err := watch.LoadState()
			if err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			client, err := api.NewClient(cfg.RefreshToken, cfg.AmazonDomain)
			if err != nil {
				return err
			}

			runner := &watch.Runner{
				Client: client,
				Config: pollerCfg,
				State:  state,
			}

			fmt.Printf("Running Alexa poller with %d route(s) every %s\n", len(pollerCfg.Routes), pollerCfg.PollEvery())
			return runner.Run(context.Background())
		},
	}
	return cmd
}

func parseDestination(value string) (watch.Action, error) {
	value = strings.TrimSpace(value)
	switch {
	case value == "", value == "stdout":
		return watch.Action{Type: watch.ActionStdout}, nil
	case strings.HasPrefix(value, "openclaw"):
		action := watch.Action{Type: watch.ActionOpenClaw, Agent: "main"}
		if idx := strings.Index(value, ":"); idx >= 0 && idx+1 < len(value) {
			action.Agent = strings.TrimSpace(value[idx+1:])
			if action.Agent == "" {
				action.Agent = "main"
			}
		}
		return action, nil
	case strings.HasPrefix(value, "exec:"):
		command := strings.TrimSpace(strings.TrimPrefix(value, "exec:"))
		if command == "" {
			return watch.Action{}, fmt.Errorf("exec destination requires a command")
		}
		return watch.Action{Type: watch.ActionExec, Command: command}, nil
	default:
		return watch.Action{}, fmt.Errorf("unsupported destination %q", value)
	}
}

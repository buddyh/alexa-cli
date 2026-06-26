package watch

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/buddyh/alexa-cli/internal/api"
)

// Runner executes configured phrase routes against Alexa history/conversations.
type Runner struct {
	Client *api.Client
	Config *Config
	State  *State

	initialSeeded bool
}

// Run executes the watcher loop until the context is cancelled.
func (r *Runner) Run(ctx context.Context) error {
	if r.Client == nil {
		return fmt.Errorf("client is required")
	}
	if r.Config == nil {
		return fmt.Errorf("config is required")
	}
	if r.State == nil {
		r.State = NewState()
	}
	if len(r.Config.Routes) == 0 {
		return fmt.Errorf("no routes configured")
	}

	if _, err := r.Client.GetDevices(); err != nil {
		return fmt.Errorf("failed to load devices: %w", err)
	}

	if err := r.poll(true); err != nil {
		return err
	}

	ticker := time.NewTicker(r.Config.PollEvery())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.poll(false); err != nil {
				return err
			}
		}
	}
}

func (r *Runner) poll(seedOnly bool) error {
	deviceMap, err := r.deviceMap()
	if err != nil {
		return err
	}

	var firstErr error
	conversationsLoaded := false
	var conversations []api.Conversation

	for _, route := range r.Config.Routes {
		sources := sourcesForRoute(route)
		for _, source := range sources {
			switch source {
			case SourceConversation:
				if !conversationsLoaded {
					conversations, err = r.Client.GetConversations()
					conversationsLoaded = true
					if err != nil {
						if firstErr == nil {
							firstErr = fmt.Errorf("conversation poll failed: %w", err)
						}
						continue
					}
				}
				if err := r.pollConversationRoute(route, conversations, seedOnly); err != nil && firstErr == nil {
					firstErr = err
				}
			case SourceHistory:
				if err := r.pollHistoryRoute(route, deviceMap, seedOnly); err != nil && firstErr == nil {
					firstErr = err
				}
			}
		}
	}

	if err := r.State.Save(); err != nil {
		return err
	}
	if seedOnly {
		r.initialSeeded = true
	}
	return firstErr
}

func (r *Runner) pollHistoryRoute(route Route, deviceMap map[string]api.Device, seedOnly bool) error {
	end := time.Now().UnixMilli()
	start := end - int64((2*time.Minute)/time.Millisecond)

	records, err := r.Client.GetCustomerHistoryRecords(start, end)
	if err != nil {
		return err
	}

	for _, record := range records {
		key := record.RecordKey
		if _, ok := r.State.SeenHistory[key]; ok {
			continue
		}

		deviceName := record.Device
		if dev, ok := deviceMap[record.Device]; ok {
			deviceName = dev.AccountName
		}

		r.State.SeenHistory[key] = record.Timestamp
		if seedOnly || record.CustomerUtterance == "" {
			continue
		}

		event := Event{
			Source:       SourceHistory,
			RouteSource:  route.Source,
			Key:          key,
			DeviceName:   deviceName,
			DeviceSerial: record.Device,
			Timestamp:    time.UnixMilli(record.Timestamp),
			Text:         record.CustomerUtterance,
			RawText:      record.CustomerUtterance,
		}
		if err := r.handleRouteEvent(route, event); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) pollConversationRoute(route Route, conversations []api.Conversation, seedOnly bool) error {
	for _, conversation := range conversations {
		if route.Device != "" && !strings.EqualFold(route.Device, conversation.DeviceName) {
			continue
		}

		r.Client.SetConversationID(conversation.ConversationID)
		resp, err := r.Client.GetConversationFragments()
		if err != nil {
			return err
		}

		for _, fragment := range resp.Fragments {
			if fragment.Metadata.Purpose != "USER" || fragment.Content == nil {
				continue
			}
			text := fragment.Content.GetText()
			if text == "" {
				continue
			}
			if _, ok := r.State.SeenConversation[fragment.FragmentURI]; ok {
				continue
			}

			r.State.SeenConversation[fragment.FragmentURI] = conversation.ConversationID
			if seedOnly {
				continue
			}

			ts, _ := time.Parse(time.RFC3339, fragment.Timestamp)
			if ts.IsZero() {
				ts = time.Now()
			}

			event := Event{
				Source:       SourceConversation,
				RouteSource:  route.Source,
				Key:          fragment.FragmentURI,
				Conversation: conversation.ConversationID,
				DeviceName:   conversation.DeviceName,
				Timestamp:    ts,
				Text:         text,
				RawText:      text,
			}
			if err := r.handleRouteEvent(route, event); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Runner) handleRouteEvent(route Route, event Event) error {
	payload, matched := MatchEvent(route, event)
	if !matched {
		return nil
	}

	switch route.Action.Type {
	case ActionOpenClaw:
		args := []string{"agent", "--message", payload}
		if route.Action.Agent != "" {
			args = append(args, "--agent", route.Action.Agent)
		}
		cmd := exec.Command("openclaw", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("openclaw dispatch failed for route %s: %w (%s)", route.ID, err, strings.TrimSpace(string(output)))
		}
	case ActionExec:
		command := strings.ReplaceAll(route.Action.Command, "{{text}}", payload)
		cmd := exec.Command("/bin/sh", "-c", command)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("exec action failed for route %s: %w (%s)", route.ID, err, strings.TrimSpace(string(output)))
		}
	case ActionStdout:
		fmt.Printf("[%s] %s -> %s\n", event.Timestamp.Format(time.RFC3339), route.Phrase, payload)
	default:
		return fmt.Errorf("unsupported action type %q", route.Action.Type)
	}

	if route.Ack != nil && route.Ack.Text != "" {
		if err := r.speakAck(route, event); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) speakAck(route Route, event Event) error {
	deviceName := route.Device
	if deviceName == "" {
		deviceName = event.DeviceName
	}
	if deviceName == "" {
		return nil
	}

	devices, err := r.Client.GetDevices()
	if err != nil {
		return err
	}
	for i := range devices {
		if strings.EqualFold(devices[i].AccountName, deviceName) || devices[i].SerialNumber == event.DeviceSerial {
			return r.Client.SequenceCommand(&devices[i], fmt.Sprintf("speak:%q", route.Ack.Text))
		}
	}
	return nil
}

func (r *Runner) deviceMap() (map[string]api.Device, error) {
	devices, err := r.Client.GetDevices()
	if err != nil {
		return nil, err
	}
	bySerial := make(map[string]api.Device, len(devices))
	for _, device := range devices {
		bySerial[device.SerialNumber] = device
	}
	return bySerial, nil
}

func sourcesForRoute(route Route) []Source {
	switch route.Source {
	case SourceConversation:
		return []Source{SourceConversation}
	case SourceHistory:
		return []Source{SourceHistory}
	default:
		return []Source{SourceConversation, SourceHistory}
	}
}

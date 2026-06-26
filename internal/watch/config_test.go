package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMatchEventPrefix(t *testing.T) {
	route := Route{
		Phrase: "clawtto",
		Action: Action{Type: ActionStdout},
	}
	if err := route.Normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	payload, ok := MatchEvent(route, Event{
		DeviceName: "Echo Show",
		Text:       "Clawtto can you summarize my unread emails",
		RawText:    "Clawtto can you summarize my unread emails",
	})
	if !ok {
		t.Fatalf("expected route to match")
	}
	if want := "summarize my unread emails"; payload != want {
		t.Fatalf("payload = %q, want %q", payload, want)
	}
}

func TestMatchEventIgnoresMetaCommentary(t *testing.T) {
	route := Route{
		Phrase: "clawtto",
		Action: Action{Type: ActionStdout},
	}
	if err := route.Normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	if _, ok := MatchEvent(route, Event{
		Text:    "nevermind clawtto send this to openclaw",
		RawText: "nevermind clawtto send this to openclaw",
	}); ok {
		t.Fatalf("expected nevermind phrase to be ignored")
	}
}

func TestMatchEventContainsWithDeviceFilter(t *testing.T) {
	route := Route{
		Phrase: "brief me",
		Device: "Kitchen Echo",
		Match:  MatchContains,
		Action: Action{Type: ActionStdout},
	}
	if err := route.Normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	if _, ok := MatchEvent(route, Event{
		DeviceName: "Office Echo",
		Text:       "Alexa, brief me on the release",
		RawText:    "Alexa, brief me on the release",
	}); ok {
		t.Fatalf("expected mismatched device to be ignored")
	}

	payload, ok := MatchEvent(route, Event{
		DeviceName: "Kitchen Echo",
		Text:       "Alexa, brief me on the release",
		RawText:    "Alexa, brief me on the release",
	})
	if !ok {
		t.Fatalf("expected route to match")
	}
	if want := "on the release"; payload != want {
		t.Fatalf("payload = %q, want %q", payload, want)
	}
}

func TestNormalizeRouteDefaults(t *testing.T) {
	route := Route{
		Phrase: "clawtto",
		Action: Action{Type: ActionOpenClaw},
	}
	if err := route.Normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if route.Source != SourceAuto {
		t.Fatalf("source = %q, want %q", route.Source, SourceAuto)
	}
	if route.Match != MatchPrefix {
		t.Fatalf("match = %q, want %q", route.Match, MatchPrefix)
	}
	if route.Action.Agent != "main" {
		t.Fatalf("agent = %q, want main", route.Action.Agent)
	}
	if len(route.Ignore) != len(defaultIgnorePhrases) {
		t.Fatalf("ignore count = %d, want %d", len(route.Ignore), len(defaultIgnorePhrases))
	}
}

func TestNormalizeRouteValidation(t *testing.T) {
	tests := []struct {
		name  string
		route Route
	}{
		{
			name:  "missing phrase",
			route: Route{Action: Action{Type: ActionStdout}},
		},
		{
			name:  "invalid source",
			route: Route{Phrase: "clawtto", Source: "rss", Action: Action{Type: ActionStdout}},
		},
		{
			name:  "invalid match",
			route: Route{Phrase: "clawtto", Match: "regex", Action: Action{Type: ActionStdout}},
		},
		{
			name:  "exec without command",
			route: Route{Phrase: "clawtto", Action: Action{Type: ActionExec}},
		},
		{
			name:  "invalid action",
			route: Route{Phrase: "clawtto", Action: Action{Type: "webhook"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.route.Normalize(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestConfigAndStateRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &Config{
		PollInterval: "250ms",
		Routes: []Route{
			{
				Phrase: "clawtto",
				Device: "Echo Show",
				Ignore: []string{"stand down", "stand down"},
				Action: Action{Type: ActionExec, Command: "printf %s {{text}}"},
				Ack:    &Ack{Text: "Sent"},
			},
		},
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	path := filepath.Join(home, ".alexa-cli", configFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("config mode = %v, want 0600", mode)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got, want := loaded.PollEvery(), 250*time.Millisecond; got != want {
		t.Fatalf("poll interval = %v, want %v", got, want)
	}
	if len(loaded.Routes) != 1 {
		t.Fatalf("routes len = %d, want 1", len(loaded.Routes))
	}
	route := loaded.Routes[0]
	if route.ID == "" {
		t.Fatalf("route ID was not generated")
	}
	if got, want := route.Action.Command, "printf %s {{text}}"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := len(route.Ignore), len(defaultIgnorePhrases)+1; got != want {
		t.Fatalf("ignore count = %d, want %d", got, want)
	}

	state := NewState()
	state.SeenHistory["record-1"] = 123
	state.SeenConversation["fragment-1"] = "conversation-1"
	if err := state.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}
	loadedState, err := LoadState()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if got, want := loadedState.SeenHistory["record-1"], int64(123); got != want {
		t.Fatalf("seen history = %d, want %d", got, want)
	}
	if got, want := loadedState.SeenConversation["fragment-1"], "conversation-1"; got != want {
		t.Fatalf("seen conversation = %q, want %q", got, want)
	}
}

func TestPollEveryFallbacks(t *testing.T) {
	for _, interval := range []string{"", "nope", "-1s", "0s"} {
		cfg := Config{PollInterval: interval}
		if got, want := cfg.PollEvery(), 5*time.Second; got != want {
			t.Fatalf("PollEvery(%q) = %v, want %v", interval, got, want)
		}
	}
}

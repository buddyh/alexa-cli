package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/buddyh/alexa-cli/internal/watch"
)

func TestParseDestination(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    watch.Action
		wantErr bool
	}{
		{
			name:  "stdout",
			value: "stdout",
			want:  watch.Action{Type: watch.ActionStdout},
		},
		{
			name:  "empty defaults to stdout",
			value: "",
			want:  watch.Action{Type: watch.ActionStdout},
		},
		{
			name:  "openclaw default agent",
			value: "openclaw",
			want:  watch.Action{Type: watch.ActionOpenClaw, Agent: "main"},
		},
		{
			name:  "openclaw explicit agent",
			value: "openclaw:nightly",
			want:  watch.Action{Type: watch.ActionOpenClaw, Agent: "nightly"},
		},
		{
			name:  "exec command",
			value: "exec:/usr/local/bin/dispatch '{{text}}'",
			want:  watch.Action{Type: watch.ActionExec, Command: "/usr/local/bin/dispatch '{{text}}'"},
		},
		{
			name:    "empty exec command",
			value:   "exec: ",
			wantErr: true,
		},
		{
			name:    "unsupported",
			value:   "webhook:https://example.test",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDestination(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parse destination: %v", err)
			}
			if got != tt.want {
				t.Fatalf("action = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRouteCommandsManageConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := execute([]string{
		"route", "add", "clawtto",
		"--device", "Echo Show",
		"--source", "history",
		"--match", "contains",
		"--to", "stdout",
		"--ack", "On it",
		"--ignore", "stand down",
	}); err != nil {
		t.Fatalf("route add: %v", err)
	}

	routes := loadRoutesForTest(t, home)
	if len(routes) != 1 {
		t.Fatalf("routes len = %d, want 1", len(routes))
	}
	route := routes[0]
	if route.ID == "" {
		t.Fatalf("route ID was not generated")
	}
	if got, want := route.Phrase, "clawtto"; got != want {
		t.Fatalf("phrase = %q, want %q", got, want)
	}
	if got, want := route.Device, "Echo Show"; got != want {
		t.Fatalf("device = %q, want %q", got, want)
	}
	if got, want := route.Source, watch.SourceHistory; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	if got, want := route.Match, watch.MatchContains; got != want {
		t.Fatalf("match = %q, want %q", got, want)
	}
	if got, want := route.Action.Type, watch.ActionStdout; got != want {
		t.Fatalf("action type = %q, want %q", got, want)
	}
	if route.Ack == nil || route.Ack.Text != "On it" {
		t.Fatalf("ack = %#v, want text", route.Ack)
	}

	if err := execute([]string{"route", "list", "--json"}); err != nil {
		t.Fatalf("route list: %v", err)
	}
	if err := execute([]string{"route", "remove", route.ID}); err != nil {
		t.Fatalf("route remove: %v", err)
	}
	routes = loadRoutesForTest(t, home)
	if len(routes) != 0 {
		t.Fatalf("routes len after remove = %d, want 0", len(routes))
	}
}

func TestRouteRemoveMissingRouteFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := execute([]string{"route", "remove", "route-missing"}); err == nil {
		t.Fatalf("expected missing route error")
	}
}

func loadRoutesForTest(t *testing.T, home string) []watch.Route {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(home, ".alexa-cli", "poller.json"))
	if err != nil {
		t.Fatalf("read poller config: %v", err)
	}
	var cfg watch.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse poller config: %v", err)
	}
	return cfg.Routes
}

package watch

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buddyh/alexa-cli/internal/config"
)

const (
	configFileName = "poller.json"
	stateFileName  = "poller-state.json"
)

var defaultIgnorePhrases = []string{
	"nevermind",
	"ignore this",
}

// Config holds the poller routes and runtime defaults.
type Config struct {
	PollInterval string  `json:"pollInterval,omitempty"`
	Routes       []Route `json:"routes"`
}

// Route describes one activation phrase route.
type Route struct {
	ID     string   `json:"id"`
	Phrase string   `json:"phrase"`
	Device string   `json:"device,omitempty"`
	Source Source   `json:"source,omitempty"`
	Match  Match    `json:"match,omitempty"`
	Ignore []string `json:"ignore,omitempty"`
	Action Action   `json:"action"`
	Ack    *Ack     `json:"ack,omitempty"`
}

// Source identifies the watcher backend.
type Source string

const (
	SourceAuto         Source = "auto"
	SourceHistory      Source = "history"
	SourceConversation Source = "conversation"
)

// Match identifies how a route phrase should be matched.
type Match string

const (
	MatchPrefix   Match = "prefix"
	MatchContains Match = "contains"
)

// Action is the destination invoked for a matched route.
type Action struct {
	Type    ActionType `json:"type"`
	Agent   string     `json:"agent,omitempty"`
	Command string     `json:"command,omitempty"`
}

// ActionType identifies the supported route actions.
type ActionType string

const (
	ActionOpenClaw ActionType = "openclaw"
	ActionExec     ActionType = "exec"
	ActionStdout   ActionType = "stdout"
)

// Ack configures an optional Alexa acknowledgement.
type Ack struct {
	Text string `json:"text"`
}

// State tracks seen records so the poller can survive restarts.
type State struct {
	SeenHistory      map[string]int64  `json:"seenHistory"`
	SeenConversation map[string]string `json:"seenConversation"`
}

// Event is a normalized match candidate emitted by a watcher backend.
type Event struct {
	Source       Source
	RouteSource  Source
	Key          string
	Conversation string
	DeviceName   string
	DeviceSerial string
	Timestamp    time.Time
	Text         string
	RawText      string
}

// ConfigPath returns the poller config path.
func ConfigPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// StatePath returns the poller state path.
func StatePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFileName), nil
}

// LoadConfig reads the poller config from disk. Missing config is valid.
func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read poller config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse poller config: %w", err)
	}

	for i := range cfg.Routes {
		if err := cfg.Routes[i].Normalize(); err != nil {
			return nil, fmt.Errorf("route %d: %w", i+1, err)
		}
	}

	return &cfg, nil
}

// SaveConfig writes the poller config to disk.
func SaveConfig(cfg *Config) error {
	for i := range cfg.Routes {
		if err := cfg.Routes[i].Normalize(); err != nil {
			return fmt.Errorf("route %d: %w", i+1, err)
		}
	}

	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal poller config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write poller config: %w", err)
	}
	return nil
}

// LoadState reads the persisted watcher state from disk.
func LoadState() (*State, error) {
	path, err := StatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), nil
		}
		return nil, fmt.Errorf("failed to read poller state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse poller state: %w", err)
	}

	if state.SeenHistory == nil {
		state.SeenHistory = make(map[string]int64)
	}
	if state.SeenConversation == nil {
		state.SeenConversation = make(map[string]string)
	}

	return &state, nil
}

// Save writes the state file to disk.
func (s *State) Save() error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path, err := StatePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal poller state: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write poller state: %w", err)
	}
	return nil
}

// NewState returns an initialized empty state.
func NewState() *State {
	return &State{
		SeenHistory:      make(map[string]int64),
		SeenConversation: make(map[string]string),
	}
}

// PollEvery returns the configured poll interval.
func (c *Config) PollEvery() time.Duration {
	if c.PollInterval == "" {
		return 5 * time.Second
	}
	d, err := time.ParseDuration(c.PollInterval)
	if err != nil || d <= 0 {
		return 5 * time.Second
	}
	return d
}

// Normalize validates and fills route defaults.
func (r *Route) Normalize() error {
	r.Phrase = strings.TrimSpace(r.Phrase)
	if r.Phrase == "" {
		return fmt.Errorf("phrase is required")
	}
	if r.ID == "" {
		r.ID = newRouteID()
	}

	if r.Source == "" {
		r.Source = SourceAuto
	}
	switch r.Source {
	case SourceAuto, SourceHistory, SourceConversation:
	default:
		return fmt.Errorf("unsupported source %q", r.Source)
	}

	if r.Match == "" {
		r.Match = MatchPrefix
	}
	switch r.Match {
	case MatchPrefix, MatchContains:
	default:
		return fmt.Errorf("unsupported match mode %q", r.Match)
	}

	if r.Action.Type == "" {
		r.Action.Type = ActionOpenClaw
	}
	switch r.Action.Type {
	case ActionOpenClaw:
	case ActionExec:
		if strings.TrimSpace(r.Action.Command) == "" {
			return fmt.Errorf("exec action requires a command")
		}
	case ActionStdout:
	default:
		return fmt.Errorf("unsupported action type %q", r.Action.Type)
	}

	r.Device = strings.TrimSpace(r.Device)
	normalizedIgnore := make([]string, 0, len(defaultIgnorePhrases)+len(r.Ignore))
	seen := make(map[string]struct{})
	for _, phrase := range append(defaultIgnorePhrases, r.Ignore...) {
		phrase = normalizePhrase(phrase)
		if phrase == "" {
			continue
		}
		if _, ok := seen[phrase]; ok {
			continue
		}
		seen[phrase] = struct{}{}
		normalizedIgnore = append(normalizedIgnore, phrase)
	}
	r.Ignore = normalizedIgnore

	if r.Action.Type == ActionOpenClaw && strings.TrimSpace(r.Action.Agent) == "" {
		r.Action.Agent = "main"
	}

	if r.Ack != nil {
		r.Ack.Text = strings.TrimSpace(r.Ack.Text)
		if r.Ack.Text == "" {
			r.Ack = nil
		}
	}

	return nil
}

func newRouteID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err == nil {
		return "route-" + hex.EncodeToString(buf)
	}
	return fmt.Sprintf("route-%d", time.Now().UnixNano())
}

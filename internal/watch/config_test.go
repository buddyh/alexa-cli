package watch

import "testing"

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
}

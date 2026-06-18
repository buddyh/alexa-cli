package watch

import (
	"strings"
	"unicode"
)

// MatchEvent reports whether the event activates the route and returns the payload.
func MatchEvent(route Route, event Event) (string, bool) {
	if route.Device != "" && !strings.EqualFold(route.Device, event.DeviceName) {
		return "", false
	}

	normalizedRaw := normalizePhrase(event.RawText)
	for _, ignore := range route.Ignore {
		if normalizedRaw == ignore || strings.HasPrefix(normalizedRaw, ignore+" ") {
			return "", false
		}
	}

	normalizedText := normalizePhrase(event.Text)
	normalizedPhrase := normalizePhrase(route.Phrase)

	var payload string
	switch route.Match {
	case MatchContains:
		idx := strings.Index(normalizedText, normalizedPhrase)
		if idx < 0 {
			return "", false
		}
		payload = strings.TrimSpace(normalizedText[idx+len(normalizedPhrase):])
	default:
		if normalizedText == normalizedPhrase {
			payload = ""
			break
		}
		if !strings.HasPrefix(normalizedText, normalizedPhrase+" ") {
			return "", false
		}
		payload = strings.TrimSpace(normalizedText[len(normalizedPhrase):])
	}

	payload = trimFiller(payload)
	if payload == "" {
		return "", false
	}
	return payload, true
}

func normalizePhrase(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastSpace := true
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r):
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		case strings.ContainsRune("',.!?:;\"-_/()", r):
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func trimFiller(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{
		"please ",
		"can you ",
		"could you ",
		"would you ",
		"hey ",
		"um ",
		"uh ",
	} {
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
		}
	}
	return value
}

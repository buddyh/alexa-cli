package api

import "testing"

func TestAcceptLanguageHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		locale string
		want   string
	}{
		{
			name:   "regional locale keeps language fallback",
			locale: "it-IT",
			want:   "it-IT,it;q=0.9,en-US;q=0.8,en;q=0.7",
		},
		{
			name:   "english locale keeps country-specific value",
			locale: "en-GB",
			want:   "en-GB,en;q=0.9,en-US;q=0.8,en;q=0.7",
		},
		{
			name:   "empty locale falls back to US english",
			locale: "",
			want:   "en-US,en;q=0.9",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := acceptLanguageHeader(tt.locale); got != tt.want {
				t.Fatalf("acceptLanguageHeader(%q) = %q, want %q", tt.locale, got, tt.want)
			}
		})
	}
}

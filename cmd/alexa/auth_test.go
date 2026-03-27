package main

import "testing"

func TestNormalizeLocalDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		domain  string
		country string
		want    string
	}{
		{
			name:    "uses explicit country when provided",
			domain:  "amazon.com",
			country: "amazon.it",
			want:    "amazon.it",
		},
		{
			name:   "falls back to domain when country omitted",
			domain: "amazon.de",
			want:   "amazon.de",
		},
		{
			name: "defaults to amazon.com when both omitted",
			want: "amazon.com",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeLocalDomain(tt.domain, tt.country); got != tt.want {
				t.Fatalf("normalizeLocalDomain(%q, %q) = %q, want %q", tt.domain, tt.country, got, tt.want)
			}
		})
	}
}

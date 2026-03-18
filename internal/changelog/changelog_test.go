package changelog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Run("BasicChangelog", func(t *testing.T) {
		raw := `{
			"values": [
				{
					"created": "2026-01-15T10:30:00Z",
					"author": {"emailAddress": "john@example.com", "displayName": "John"},
					"items": [
						{"field": "status", "fromString": "To Do", "toString": "In Progress"},
						{"field": "assignee", "fromString": "", "toString": "Jane"}
					]
				}
			]
		}`

		result, err := Parse("PROJ-1", []byte(raw), Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Issue != "PROJ-1" {
			t.Errorf("expected issue PROJ-1, got %s", result.Issue)
		}
		if len(result.Changes) != 2 {
			t.Fatalf("expected 2 changes, got %d", len(result.Changes))
		}

		c := result.Changes[0]
		if c.Field != "status" {
			t.Errorf("expected field 'status', got %q", c.Field)
		}
		if c.From != "To Do" {
			t.Errorf("expected from 'To Do', got %v", c.From)
		}
		if c.To != "In Progress" {
			t.Errorf("expected to 'In Progress', got %v", c.To)
		}
		if c.Author != "john@example.com" {
			t.Errorf("expected author john@example.com, got %q", c.Author)
		}

		c2 := result.Changes[1]
		if c2.From != nil {
			t.Errorf("expected nil from, got %v", c2.From)
		}
		if c2.To != "Jane" {
			t.Errorf("expected to 'Jane', got %v", c2.To)
		}
	})

	t.Run("FieldFilter", func(t *testing.T) {
		raw := `{
			"values": [
				{
					"created": "2026-01-15T10:30:00Z",
					"author": {"emailAddress": "john@example.com"},
					"items": [
						{"field": "status", "fromString": "To Do", "toString": "Done"},
						{"field": "priority", "fromString": "Low", "toString": "High"}
					]
				}
			]
		}`

		result, err := Parse("PROJ-1", []byte(raw), Options{Field: "status"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(result.Changes))
		}
		if result.Changes[0].Field != "status" {
			t.Errorf("expected field 'status', got %q", result.Changes[0].Field)
		}
	})

	t.Run("FieldFilterCaseInsensitive", func(t *testing.T) {
		raw := `{
			"values": [
				{
					"created": "2026-01-15T10:30:00Z",
					"author": {"emailAddress": "john@example.com"},
					"items": [
						{"field": "Status", "fromString": "To Do", "toString": "Done"}
					]
				}
			]
		}`

		result, err := Parse("PROJ-1", []byte(raw), Options{Field: "status"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(result.Changes))
		}
	})

	t.Run("SinceISODate", func(t *testing.T) {
		raw := `{
			"values": [
				{
					"created": "2025-06-01T10:00:00Z",
					"author": {"emailAddress": "old@example.com"},
					"items": [{"field": "status", "fromString": "A", "toString": "B"}]
				},
				{
					"created": "2026-03-01T10:00:00Z",
					"author": {"emailAddress": "new@example.com"},
					"items": [{"field": "status", "fromString": "B", "toString": "C"}]
				}
			]
		}`

		result, err := Parse("PROJ-1", []byte(raw), Options{Since: "2026-01-01"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Changes) != 1 {
			t.Fatalf("expected 1 change after 2026-01-01, got %d", len(result.Changes))
		}
		if result.Changes[0].Author != "new@example.com" {
			t.Errorf("expected new@example.com, got %q", result.Changes[0].Author)
		}
	})

	t.Run("SinceDuration", func(t *testing.T) {
		// Pin "now" to 2026-03-18T12:00:00Z. "2h" means since 10:00:00Z.
		now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

		raw := `{
			"values": [
				{
					"created": "2026-03-18T09:00:00Z",
					"author": {"emailAddress": "old@example.com"},
					"items": [{"field": "status", "fromString": "A", "toString": "B"}]
				},
				{
					"created": "2026-03-18T11:00:00Z",
					"author": {"emailAddress": "recent@example.com"},
					"items": [{"field": "status", "fromString": "B", "toString": "C"}]
				}
			]
		}`

		result, err := Parse("PROJ-1", []byte(raw), Options{Since: "2h", Now: now})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Changes) != 1 {
			t.Fatalf("expected 1 change in last 2h, got %d", len(result.Changes))
		}
		if result.Changes[0].Author != "recent@example.com" {
			t.Errorf("expected recent@example.com, got %q", result.Changes[0].Author)
		}
	})

	t.Run("EmptyChangelog", func(t *testing.T) {
		raw := `{"values": []}`

		result, err := Parse("PROJ-1", []byte(raw), Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Changes) != 0 {
			t.Errorf("expected 0 changes, got %d", len(result.Changes))
		}
	})

	t.Run("AuthorFallbackToDisplayName", func(t *testing.T) {
		raw := `{
			"values": [
				{
					"created": "2026-01-15T10:30:00Z",
					"author": {"displayName": "John Doe"},
					"items": [{"field": "status", "fromString": "A", "toString": "B"}]
				}
			]
		}`

		result, err := Parse("PROJ-1", []byte(raw), Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Changes[0].Author != "John Doe" {
			t.Errorf("expected author 'John Doe', got %q", result.Changes[0].Author)
		}
	})

	t.Run("JSONOutputValid", func(t *testing.T) {
		raw := `{
			"values": [
				{
					"created": "2026-01-15T10:30:00Z",
					"author": {"emailAddress": "john@example.com"},
					"items": [
						{"field": "status", "fromString": "To Do", "toString": "In Progress"}
					]
				}
			]
		}`

		result, err := Parse("PROJ-123", []byte(raw), Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("output is not valid JSON: %v", err)
		}
		if parsed["issue"] != "PROJ-123" {
			t.Errorf("expected issue PROJ-123 in JSON output")
		}
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		_, err := Parse("PROJ-1", []byte("not json"), Options{})
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("InvalidSince", func(t *testing.T) {
		raw := `{"values": []}`
		_, err := Parse("PROJ-1", []byte(raw), Options{Since: "not-a-date-or-duration"})
		if err == nil {
			t.Error("expected error for invalid --since value")
		}
	})

	t.Run("NullToWhenToStringEmpty", func(t *testing.T) {
		raw := `{
			"values": [
				{
					"created": "2026-01-15T10:30:00Z",
					"author": {"emailAddress": "john@example.com"},
					"items": [
						{"field": "assignee", "fromString": "Alice", "toString": ""}
					]
				}
			]
		}`

		result, err := Parse("PROJ-1", []byte(raw), Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(result.Changes))
		}
		c := result.Changes[0]
		if c.From != "Alice" {
			t.Errorf("expected from 'Alice', got %v", c.From)
		}
		if c.To != nil {
			t.Errorf("expected nil to (toString was empty), got %v", c.To)
		}

		// Verify JSON output has "to": null.
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}
		if !strings.Contains(string(data), `"to":null`) {
			t.Errorf("expected JSON to contain '\"to\":null', got %s", data)
		}
	})
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, got time.Time)
	}{
		{
			name:  "ISO date",
			input: "2026-01-01",
			check: func(t *testing.T, got time.Time) {
				want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "RFC3339",
			input: "2026-01-15T10:30:00Z",
			check: func(t *testing.T, got time.Time) {
				want := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "2h duration",
			input: "2h",
			check: func(t *testing.T, got time.Time) {
				want := now.Add(-2 * time.Hour)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "1d duration (8h Jira convention)",
			input: "1d",
			check: func(t *testing.T, got time.Time) {
				want := now.Add(-8 * time.Hour)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "30m duration",
			input: "30m",
			check: func(t *testing.T, got time.Time) {
				want := now.Add(-30 * time.Minute)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "compound 1d 3h",
			input: "1d 3h",
			check: func(t *testing.T, got time.Time) {
				want := now.Add(-(8 + 3) * time.Hour) // 1d=8h + 3h
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			// ISO datetime without timezone (layout "2006-01-02T15:04:05").
			name:  "ISO datetime no TZ",
			input: "2026-01-15T10:30:00",
			check: func(t *testing.T, got time.Time) {
				want := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:    "invalid string",
			input:   "not-valid",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSince(tt.input, now)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseSince(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSince(%q) unexpected error: %v", tt.input, err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// TestParseSince_ZeroNow verifies that parseSince falls back to time.Now()
// when the now parameter is the zero value.
func TestParseSince_ZeroNow(t *testing.T) {
	before := time.Now()
	got, err := parseSince("2h", time.Time{})
	after := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// got should be approximately 2 hours before now; verify the bounds.
	lowerBound := before.Add(-2*time.Hour - time.Second)
	upperBound := after.Add(-2 * time.Hour)
	// The result must be after lowerBound and not after upperBound+1s to
	// account for execution time between before/after measurements.
	upperBoundWithSlack := after.Add(-2*time.Hour + time.Second)
	if got.Before(lowerBound) || got.After(upperBoundWithSlack) {
		t.Errorf("parseSince(\"2h\", zero) = %v, want ~2h before now (window: %v .. %v)",
			got, lowerBound, upperBound)
	}
}

// TestParse_UnparseableTimestampSkipped verifies that entries with timestamps
// that cannot be parsed by parseJiraTime are silently skipped (via continue)
// when a Since filter is active, while valid entries are still returned.
func TestParse_UnparseableTimestampSkipped(t *testing.T) {
	raw := `{
		"values": [
			{
				"created": "not-a-date",
				"author": {"emailAddress": "bad@example.com"},
				"items": [{"field": "status", "fromString": "A", "toString": "B"}]
			},
			{
				"created": "2026-01-15T10:30:00Z",
				"author": {"emailAddress": "good@example.com"},
				"items": [{"field": "status", "fromString": "B", "toString": "C"}]
			}
		]
	}`

	result, err := Parse("PROJ-1", []byte(raw), Options{Since: "2025-01-01"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The entry with an unparseable timestamp must be skipped.
	if len(result.Changes) != 1 {
		t.Fatalf("expected 1 change (bad timestamp skipped), got %d", len(result.Changes))
	}
	if result.Changes[0].Author != "good@example.com" {
		t.Errorf("expected good@example.com, got %q", result.Changes[0].Author)
	}
}

// TestParseJiraTime_Formats exercises all timestamp formats that parseJiraTime
// must accept, including the millisecond-offset format that Jira uses in
// practice, and verifies that a completely unparseable string returns an error.
func TestParseJiraTime_Formats(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"RFC3339", "2026-01-15T10:30:00Z", false},
		// Millisecond with numeric timezone offset — layout "2006-01-02T15:04:05.000-0700".
		{"MillisOffset", "2026-01-15T10:30:00.000+0900", false},
		// Millisecond with literal Z suffix — layout "2006-01-02T15:04:05.000Z".
		{"MillisZ", "2026-01-15T10:30:00.000Z", false},
		// Plain seconds with Z — layout "2006-01-02T15:04:05Z".
		{"SecondsZ", "2026-01-15T10:30:00Z", false},
		{"Invalid", "not-a-timestamp", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseJiraTime(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("parseJiraTime(%q) expected error, got nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parseJiraTime(%q) unexpected error: %v", tt.input, err)
			}
		})
	}
}

package duration

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"30m", 1800, false},
		{"2h", 7200, false},
		{"1d", 28800, false},      // 8h
		{"1w", 144000, false},     // 5d = 40h
		{"1d 3h", 39600, false},   // 8h + 3h = 11h
		{"2h 30m", 9000, false},   // 2.5h
		{"1w 2d 3h 15m", 213300, false}, // 40h + 16h + 3h + 15m
		{"", 0, true},
		{"abc", 0, true},
		{"0h", 0, false},
		{"10m", 600, false},
		// Bug fix: garbage text around valid units must be rejected.
		{"2h garbage", 0, true},
		{"abc2h", 0, true},
		{"2hx", 0, true},
		{"2hours", 0, true},
		{"hello 2h world", 0, true},
		{"2h 30m extra", 0, true},
		{" 2h ", 7200, false}, // leading/trailing whitespace is OK
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

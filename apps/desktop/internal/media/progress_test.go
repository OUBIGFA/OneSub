package media

import "testing"

func TestComposeProgressUsesRealTimestamp(t *testing.T) {
	for _, tc := range []struct {
		line     string
		duration int64
		value    float64
		ok       bool
	}{
		{"out_time_us=2500000", 10000, .25, true},
		{"out_time_us=5000000", 5000, .999, true},
		{"out_time_us=99999999", 5000, .999, true},
		{"out_time_us=N/A", 5000, 0, false},
		{"out_time_us=-1", 5000, 0, false},
		{"progress=end", 5000, 0, false},
		{"out_time_us=10", 0, 0, false},
	} {
		value, ok := composeFraction(tc.line, tc.duration)
		if value != tc.value || ok != tc.ok {
			t.Fatal(tc, value, ok)
		}
	}
}

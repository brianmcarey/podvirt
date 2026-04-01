package config

import "testing"

func TestParseQuantityBytes(t *testing.T) {
	cases := []struct {
		input string
		want  uint64
	}{
		{"1Ki", 1024},
		{"1Mi", 1024 * 1024},
		{"1Gi", 1024 * 1024 * 1024},
		{"1K", 1000},
		{"1M", 1000 * 1000},
		{"1G", 1000 * 1000 * 1000},
		{"4096", 4096},
	}

	for _, tc := range cases {
		got, err := ParseQuantityBytes(tc.input)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestParseQuantityBytes_Invalid(t *testing.T) {
	for _, input := range []string{"", "0Gi", "abc", "1TB", "-1Gi"} {
		if _, err := ParseQuantityBytes(input); err == nil {
			t.Fatalf("%q: expected error", input)
		}
	}
}

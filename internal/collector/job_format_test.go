package collector

import "testing"

func TestFormatExecutionHosts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single host",
			in:   "host-a",
			want: "host-a",
		},
		{
			name: "same host repeated",
			in:   "host-a,host-a,host-a,host-a",
			want: "4 * host-a",
		},
		{
			name: "mixed hosts",
			in:   "host-a,host-a,host-b",
			want: "2 * host-a,host-b",
		},
		{
			name: "trim empty parts",
			in:   "host-a,, host-a ",
			want: "2 * host-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatExecutionHosts(tt.in); got != tt.want {
				t.Fatalf("formatExecutionHosts(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

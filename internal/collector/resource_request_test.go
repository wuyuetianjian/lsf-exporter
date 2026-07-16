package collector

import "testing"

func TestRequestedMemoryKB(t *testing.T) {
	tests := []struct {
		name        string
		resourceReq string
		want        int64
	}{
		{
			name:        "rusage mem",
			resourceReq: "select[type==X86_64] rusage[mem=2048]",
			want:        2097152,
		},
		{
			name:        "decimal rusage mem",
			resourceReq: "rusage[mem=1.5]",
			want:        1536,
		},
		{
			name:        "no rusage mem",
			resourceReq: "select[mem>1024]",
			want:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestedMemoryKB(tt.resourceReq); got != tt.want {
				t.Fatalf("requestedMemoryKB(%q) = %d, want %d", tt.resourceReq, got, tt.want)
			}
		})
	}
}

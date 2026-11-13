package node

import "testing"

func TestResolveCommitteeCount(t *testing.T) {
	count := func(v uint64) *uint64 { return &v }
	tests := []struct {
		name      string
		flagCount uint64
		flagSet   bool
		config    *uint64
		want      uint64
	}{
		{"flag overrides config", 4, true, count(8), 4},
		{"config when flag unset", 1, false, count(8), 8},
		{"default when neither set", 1, false, nil, 1},
		{"flag wins even when equal to default", 1, true, count(8), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveCommitteeCount(tt.flagCount, tt.flagSet, tt.config); got != tt.want {
				t.Fatalf("resolveCommitteeCount = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestConfigAddressesUseJoinHostPort(t *testing.T) {
	cfg := Config{HTTPAddr: "::1", APIPort: 5052, MetricsPort: 5054}
	if got := cfg.apiAddress(); got != "[::1]:5052" {
		t.Fatalf("apiAddress=%q, want [::1]:5052", got)
	}
	if got := cfg.metricsAddress(); got != "[::1]:5054" {
		t.Fatalf("metricsAddress=%q, want [::1]:5054", got)
	}
}

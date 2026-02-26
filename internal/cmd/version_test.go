package cmd

import "testing"

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "uses provided version", in: "v1.2.3", want: "v1.2.3"},
		{name: "trim spaces", in: "  1.0.0  ", want: "1.0.0"},
		{name: "empty falls back to dev", in: "", want: "dev"},
		{name: "whitespace falls back to dev", in: "   ", want: "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveVersion(tt.in)
			if got != tt.want {
				t.Fatalf("resolveVersion(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}


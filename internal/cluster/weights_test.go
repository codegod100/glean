package cluster

import "testing"

func TestSignalToColumn(t *testing.T) {
	cases := []struct {
		signal string
		want   string
	}{
		{"sub", "w_sub"},
		{"like", "w_like"},
		{"tag", "w_tag"},
		{"social", "w_social"},
		{"pop", "w_pop"},
		{"category", "w_category"},
		{"content", "w_content"},
		{"unknown", ""},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.signal, func(t *testing.T) {
			if got := signalToColumn(c.signal); got != c.want {
				t.Fatalf("signalToColumn(%q) = %q, want %q", c.signal, got, c.want)
			}
		})
	}
}

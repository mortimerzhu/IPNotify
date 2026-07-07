package local

import "testing"

func TestDiff(t *testing.T) {
	tests := []struct {
		name       string
		prev, cur  map[string][]string
		wantIfaces map[string][]string // interface -> expected New
	}{
		{
			name:       "no change",
			prev:       map[string][]string{"en0": {"192.168.1.2"}},
			cur:        map[string][]string{"en0": {"192.168.1.2"}},
			wantIfaces: map[string][]string{},
		},
		{
			name:       "address changed",
			prev:       map[string][]string{"en0": {"192.168.1.2"}},
			cur:        map[string][]string{"en0": {"192.168.1.3"}},
			wantIfaces: map[string][]string{"en0": {"192.168.1.3"}},
		},
		{
			name:       "interface appeared",
			prev:       map[string][]string{},
			cur:        map[string][]string{"en0": {"10.0.0.1"}},
			wantIfaces: map[string][]string{"en0": {"10.0.0.1"}},
		},
		{
			name:       "interface disappeared",
			prev:       map[string][]string{"en0": {"10.0.0.1"}},
			cur:        map[string][]string{},
			wantIfaces: map[string][]string{"en0": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := diff(tt.prev, tt.cur, "host")
			if len(events) != len(tt.wantIfaces) {
				t.Fatalf("got %d events, want %d", len(events), len(tt.wantIfaces))
			}
			for _, e := range events {
				want, ok := tt.wantIfaces[e.Interface]
				if !ok {
					t.Errorf("unexpected event for %s", e.Interface)
					continue
				}
				if !equal(e.New, want) {
					t.Errorf("%s New = %v, want %v", e.Interface, e.New, want)
				}
			}
		})
	}
}

package server

import "testing"

func TestListenAddr(t *testing.T) {
	tests := []struct {
		name              string
		release, dev, lan bool
		want              string
	}{
		{name: "release", release: true, want: ":55555"},
		{name: "release with --lan", release: true, lan: true, want: ":55555"},
		{name: "release with --dev", release: true, dev: true, want: "127.0.0.1:55555"},
		{name: "release with --dev --lan", release: true, dev: true, lan: true, want: ":55555"},
		{name: "dev build", want: "127.0.0.1:55555"},
		{name: "dev build with --lan", lan: true, want: ":55555"},
		{name: "dev build with --dev", dev: true, want: "127.0.0.1:55555"},
		{name: "dev build with --dev --lan", dev: true, lan: true, want: ":55555"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ListenAddr("55555", tt.release, tt.dev, tt.lan); got != tt.want {
				t.Errorf("ListenAddr(release=%v, dev=%v, lan=%v) = %q, want %q", tt.release, tt.dev, tt.lan, got, tt.want)
			}
		})
	}
}

package monitor

import "testing"

func TestActionRunsOnTarget(t *testing.T) {
	tests := []struct {
		name   string
		target ActionTarget
		cloud  bool
		want   bool
	}{
		{name: "default local", cloud: false, want: true},
		{name: "default cloud", cloud: true, want: true},
		{name: "both local", target: ActionBoth, cloud: false, want: true},
		{name: "both cloud", target: ActionBoth, cloud: true, want: true},
		{name: "local on local", target: ActionLocal, cloud: false, want: true},
		{name: "local on cloud", target: ActionLocal, cloud: true, want: false},
		{name: "cloud on local", target: ActionCloud, cloud: false, want: false},
		{name: "cloud on cloud", target: ActionCloud, cloud: true, want: true},
		{name: "invalid", target: "invalid", cloud: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Action{Target: tt.target}).runsOn(tt.cloud); got != tt.want {
				t.Fatalf("runsOn(%v) = %v, want %v", tt.cloud, got, tt.want)
			}
		})
	}
}

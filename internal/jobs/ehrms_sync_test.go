package jobs

import (
	"testing"
	"time"
)

func TestNextEHRMSSyncTime(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "before morning run",
			now:  time.Date(2026, 7, 22, 7, 59, 0, 0, ehrmsSyncLocation),
			want: time.Date(2026, 7, 22, 8, 0, 0, 0, ehrmsSyncLocation),
		},
		{
			name: "after morning run",
			now:  time.Date(2026, 7, 22, 8, 1, 0, 0, ehrmsSyncLocation),
			want: time.Date(2026, 7, 22, 20, 0, 0, 0, ehrmsSyncLocation),
		},
		{
			name: "after evening run",
			now:  time.Date(2026, 7, 22, 20, 1, 0, 0, ehrmsSyncLocation),
			want: time.Date(2026, 7, 23, 8, 0, 0, 0, ehrmsSyncLocation),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextEHRMSSyncTime(tt.now); !got.Equal(tt.want) {
				t.Fatalf("next sync = %s, want %s", got, tt.want)
			}
		})
	}
}

package jobs

import (
	"testing"
	"time"
)

func TestNextEHRMSDailyCatalogSyncTime(t *testing.T) {
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
			want: time.Date(2026, 7, 23, 8, 0, 0, 0, ehrmsSyncLocation),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextEHRMSDailyCatalogSyncTime(tt.now); !got.Equal(tt.want) {
				t.Fatalf("next sync = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNextEHRMSHalfHourSyncTime(t *testing.T) {
	tests := []struct {
		now  time.Time
		want time.Time
	}{
		{
			now:  time.Date(2026, 7, 22, 8, 1, 0, 0, ehrmsSyncLocation),
			want: time.Date(2026, 7, 22, 8, 30, 0, 0, ehrmsSyncLocation),
		},
		{
			now:  time.Date(2026, 7, 22, 8, 30, 0, 0, ehrmsSyncLocation),
			want: time.Date(2026, 7, 22, 9, 0, 0, 0, ehrmsSyncLocation),
		},
		{
			now:  time.Date(2026, 7, 22, 23, 59, 0, 0, ehrmsSyncLocation),
			want: time.Date(2026, 7, 23, 0, 0, 0, 0, ehrmsSyncLocation),
		},
	}
	for _, tt := range tests {
		if got := nextEHRMSHalfHourSyncTime(tt.now); !got.Equal(tt.want) {
			t.Fatalf("next half-hour sync = %s, want %s", got, tt.want)
		}
	}
}

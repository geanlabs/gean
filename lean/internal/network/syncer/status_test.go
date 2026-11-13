package syncer

import "testing"

func TestSyncStatus_String(t *testing.T) {
	cases := []struct {
		s    SyncStatus
		want string
	}{
		{SyncIdle, "idle"},
		{SyncSyncing, "syncing"},
		{SyncSynced, "synced"},
		{SyncStatus(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("SyncStatus(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}

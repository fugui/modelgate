package quota

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDailySessionTracker_BasicFlow(t *testing.T) {
	tracker := NewDailySessionTracker()
	now := time.Now()
	userID1 := "user-1"
	userID2 := "user-2"

	// user 1 has 3 requests with session A, 2 requests with session B, 2 no-session requests
	tracker.RecordRequest(userID1, "hdr:sess-A", now)
	tracker.RecordRequest(userID1, "hdr:sess-A", now)
	tracker.RecordRequest(userID1, "hdr:sess-A", now)
	tracker.RecordRequest(userID1, "hdr:sess-B", now)
	tracker.RecordRequest(userID1, "hdr:sess-B", now)
	tracker.RecordRequest(userID1, "", now)
	tracker.RecordRequest(userID1, "", now)

	// user 2 has 4 requests with session C, 1 no-session request
	tracker.RecordRequest(userID2, "body:conv:sess-C", now)
	tracker.RecordRequest(userID2, "body:conv:sess-C", now)
	tracker.RecordRequest(userID2, "body:conv:sess-C", now)
	tracker.RecordRequest(userID2, "body:conv:sess-C", now)
	tracker.RecordRequest(userID2, "", now)

	// Verify User 1 stats
	u1Sessions, u1SessionReqs, u1NoSessionReqs := tracker.GetUserStats(userID1, now)
	assert.Equal(t, 2, u1Sessions, "User 1 should have 2 distinct sessions (A, B)")
	assert.Equal(t, 5, u1SessionReqs, "User 1 should have 5 session requests (3+2)")
	assert.Equal(t, 2, u1NoSessionReqs, "User 1 should have 2 no-session requests")

	// Verify User 2 stats
	u2Sessions, u2SessionReqs, u2NoSessionReqs := tracker.GetUserStats(userID2, now)
	assert.Equal(t, 1, u2Sessions, "User 2 should have 1 distinct session (C)")
	assert.Equal(t, 4, u2SessionReqs, "User 2 should have 4 session requests")
	assert.Equal(t, 1, u2NoSessionReqs, "User 2 should have 1 no-session request")

	// Verify Global stats
	totalSessions, totalSessionReqs, totalNoSessionReqs := tracker.GetGlobalStats(now)
	assert.Equal(t, 3, totalSessions, "Total distinct sessions across users should be 3 (A, B, C)")
	assert.Equal(t, 9, totalSessionReqs, "Total session requests should be 9 (5+4)")
	assert.Equal(t, 3, totalNoSessionReqs, "Total no-session requests should be 3 (2+1)")
}

func TestDailySessionTracker_CrossDayReset(t *testing.T) {
	tracker := NewDailySessionTracker()
	day1 := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	userID := "user-1"

	// Record on day 1
	tracker.RecordRequest(userID, "hdr:sess-1", day1)
	tracker.RecordRequest(userID, "hdr:sess-2", day1)
	tracker.RecordRequest(userID, "", day1)

	d1Sessions, d1SessionReqs, d1NoSessionReqs := tracker.GetUserStats(userID, day1)
	assert.Equal(t, 2, d1Sessions)
	assert.Equal(t, 2, d1SessionReqs)
	assert.Equal(t, 1, d1NoSessionReqs)

	// Record on day 2 (auto-resets for day 2)
	tracker.RecordRequest(userID, "hdr:sess-3", day2)
	d2Sessions, d2SessionReqs, d2NoSessionReqs := tracker.GetUserStats(userID, day2)
	assert.Equal(t, 1, d2Sessions, "On day 2, distinct sessions should be 1")
	assert.Equal(t, 1, d2SessionReqs)
	assert.Equal(t, 0, d2NoSessionReqs)

	// CleanupExpired
	tracker.CleanupExpired(day2)
	assert.Equal(t, 1, len(tracker.stats))
}

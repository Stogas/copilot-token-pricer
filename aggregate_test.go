package main

import (
	"testing"
	"time"
)

func TestFilterEventsByRecentMonthPeriods(t *testing.T) {
	loc := time.Local
	events := []RequestEvent{
		{ModelID: "old", TimestampMs: time.Date(2026, time.February, 28, 23, 59, 0, 0, loc).UnixMilli()},
		{ModelID: "previous", TimestampMs: time.Date(2026, time.March, 1, 0, 0, 0, 0, loc).UnixMilli()},
		{ModelID: "current", TimestampMs: time.Date(2026, time.April, 15, 12, 0, 0, 0, loc).UnixMilli()},
		{ModelID: "future", TimestampMs: time.Date(2026, time.May, 1, 0, 0, 0, 0, loc).UnixMilli()},
		{ModelID: "unknown"},
	}

	filtered := filterEventsByRecentPeriodsAt(events, "month", 2, time.Date(2026, time.April, 28, 12, 0, 0, 0, loc))
	if len(filtered) != 2 {
		t.Fatalf("expected 2 events, got %d", len(filtered))
	}
	if filtered[0].ModelID != "previous" || filtered[1].ModelID != "current" {
		t.Fatalf("unexpected filtered events: %+v", filtered)
	}
}

func TestFilterEventsByRecentWeekPeriods(t *testing.T) {
	loc := time.Local
	events := []RequestEvent{
		{ModelID: "old", TimestampMs: time.Date(2026, time.April, 12, 23, 59, 0, 0, loc).UnixMilli()},
		{ModelID: "previous", TimestampMs: time.Date(2026, time.April, 13, 0, 0, 0, 0, loc).UnixMilli()},
		{ModelID: "current", TimestampMs: time.Date(2026, time.April, 28, 12, 0, 0, 0, loc).UnixMilli()},
		{ModelID: "future", TimestampMs: time.Date(2026, time.May, 4, 0, 0, 0, 0, loc).UnixMilli()},
	}

	filtered := filterEventsByRecentPeriodsAt(events, "week", 3, time.Date(2026, time.April, 28, 12, 0, 0, 0, loc))
	if len(filtered) != 2 {
		t.Fatalf("expected 2 events, got %d", len(filtered))
	}
	if filtered[0].ModelID != "previous" || filtered[1].ModelID != "current" {
		t.Fatalf("unexpected filtered events: %+v", filtered)
	}
}

package main

import (
	"fmt"
	"sort"
	"time"
)

type AggregateRow struct {
	Period       string
	ModelID      string
	Requests     int
	PromptTokens int64
	OutputTokens int64
}

func aggregateEvents(events []RequestEvent, period string) []AggregateRow {
	type key struct {
		period string
		model  string
	}

	rowsByKey := make(map[key]*AggregateRow)
	for _, event := range events {
		rowKey := key{
			period: periodLabel(event.TimestampMs, period),
			model:  event.ModelID,
		}
		if rowKey.model == "" {
			rowKey.model = "unknown"
		}

		row := rowsByKey[rowKey]
		if row == nil {
			row = &AggregateRow{Period: rowKey.period, ModelID: rowKey.model}
			rowsByKey[rowKey] = row
		}
		row.Requests++
		row.PromptTokens += event.PromptTokens
		row.OutputTokens += event.OutputTokens
	}

	rows := make([]AggregateRow, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		rows = append(rows, *row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Period != rows[j].Period {
			return rows[i].Period < rows[j].Period
		}
		leftTotal := rows[i].PromptTokens + rows[i].OutputTokens
		rightTotal := rows[j].PromptTokens + rows[j].OutputTokens
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return rows[i].ModelID < rows[j].ModelID
	})

	return rows
}

func filterEventsByRecentPeriods(events []RequestEvent, period string, periods int) []RequestEvent {
	return filterEventsByRecentPeriodsAt(events, period, periods, time.Now())
}

func filterEventsByRecentPeriodsAt(events []RequestEvent, period string, periods int, now time.Time) []RequestEvent {
	if periods == 0 || period == "none" {
		return events
	}

	start, end := recentPeriodWindow(now, period, periods)
	filtered := make([]RequestEvent, 0, len(events))
	for _, event := range events {
		if event.TimestampMs == 0 {
			continue
		}

		t := time.UnixMilli(event.TimestampMs).Local()
		if !t.Before(start) && t.Before(end) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func recentPeriodWindow(now time.Time, period string, periods int) (time.Time, time.Time) {
	if periods < 1 {
		periods = 1
	}

	now = now.Local()
	switch period {
	case "week":
		currentStart := startOfISOWeek(now)
		return currentStart.AddDate(0, 0, -7*(periods-1)), currentStart.AddDate(0, 0, 7)
	case "month":
		currentStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return currentStart.AddDate(0, -(periods - 1), 0), currentStart.AddDate(0, 1, 0)
	default:
		return time.Time{}, time.Time{}
	}
}

func startOfISOWeek(t time.Time) time.Time {
	t = t.Local()
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	weekday := int(start.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return start.AddDate(0, 0, -(weekday - 1))
}

func periodLabel(timestampMs int64, period string) string {
	if period == "none" {
		return "all"
	}
	if timestampMs == 0 {
		return "unknown"
	}

	t := time.UnixMilli(timestampMs).Local()
	switch period {
	case "week":
		year, week := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", year, week)
	case "month":
		return t.Format("2006-01")
	default:
		return "all"
	}
}

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

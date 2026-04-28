package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

type ModelPrice struct {
	Name          string
	Provider      string
	InputPerMTok  float64
	OutputPerMTok float64

	PromptTokenThreshold int64
	HighInputPerMTok     float64
	HighOutputPerMTok    float64
	Notes                string
}

type CostAggregateRow struct {
	Period       string
	ModelID      string
	Requests     int
	PromptTokens int64
	OutputTokens int64
	Priced       bool
	Price        ModelPrice
	InputCost    float64
	OutputCost   float64
}

func printPricingInformation(events []RequestEvent, cost string) {
	if len(events) == 0 {
		return
	}

	models := uniqueModelsFromEvents(events)

	fmt.Printf("Pricing model: %s\n", cost)
	fmt.Println("Pricing basis: standard API token prices per 1M tokens; excludes cache discounts, batch/flex/priority modes, data residency uplifts, tool/search charges, media-specific charges, and Copilot subscription effects.")
	fmt.Println("Pricing sources: Anthropic configured public API rates; OpenAI API pricing https://developers.openai.com/api/docs/pricing and https://openai.com/api/pricing/; Gemini Developer API pricing https://ai.google.dev/gemini-api/docs/pricing (last updated 2026-04-22 UTC).")
	fmt.Println("Tiered Gemini Pro prices are calculated per request using that request's input tokens.")

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "DETECTED MODEL\tPROVIDER\tPRICING MODEL\tIN$/MTOK\tOUT$/MTOK\tNOTES")
	for _, modelID := range models {
		price, ok := priceForModel(modelID, cost)
		if !ok {
			fmt.Fprintf(writer, "%s\tn/a\tn/a\tn/a\tn/a\tno matching price for --cost %s\n", modelID, cost)
			continue
		}

		fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			modelID,
			price.Provider,
			price.Name,
			price.inputRateText(),
			price.outputRateText(),
			price.Notes,
		)
	}
	_ = writer.Flush()
	fmt.Println()
}

func printCostRows(events []RequestEvent, period string, cost string) {
	rows := aggregateEventsWithCost(events, period, cost)
	if len(rows) == 0 {
		fmt.Println("No completed request result events with token data found.")
		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if period == "none" {
		fmt.Fprintln(writer, "MODEL\tREQUESTS\tINPUT\tOUTPUT\tTOTAL\tIN$/MTOK\tOUT$/MTOK\tCOST")
	} else {
		fmt.Fprintln(writer, "PERIOD\tMODEL\tREQUESTS\tINPUT\tOUTPUT\tTOTAL\tIN$/MTOK\tOUT$/MTOK\tCOST")
	}

	var pricedInputTokens int64
	var pricedOutputTokens int64
	var unpricedInputTokens int64
	var unpricedOutputTokens int64
	var totalCost float64

	for _, row := range rows {
		costText := "n/a"
		inputRateText := "n/a"
		outputRateText := "n/a"
		if row.Priced {
			costText = formatUSD(row.InputCost + row.OutputCost)
			inputRateText = row.Price.inputRateText()
			outputRateText = row.Price.outputRateText()
			pricedInputTokens += row.PromptTokens
			pricedOutputTokens += row.OutputTokens
			totalCost += row.InputCost + row.OutputCost
		} else {
			unpricedInputTokens += row.PromptTokens
			unpricedOutputTokens += row.OutputTokens
		}

		if period == "none" {
			fmt.Fprintf(
				writer,
				"%s\t%d\t%d\t%d\t%d\t%s\t%s\t%s\n",
				row.ModelID,
				row.Requests,
				row.PromptTokens,
				row.OutputTokens,
				row.PromptTokens+row.OutputTokens,
				inputRateText,
				outputRateText,
				costText,
			)
		} else {
			fmt.Fprintf(
				writer,
				"%s\t%s\t%d\t%d\t%d\t%d\t%s\t%s\t%s\n",
				row.Period,
				row.ModelID,
				row.Requests,
				row.PromptTokens,
				row.OutputTokens,
				row.PromptTokens+row.OutputTokens,
				inputRateText,
				outputRateText,
				costText,
			)
		}
	}

	if period == "none" {
		fmt.Fprintf(writer, "PRICED TOTAL\t\t%d\t%d\t%d\t\t\t%s\n", pricedInputTokens, pricedOutputTokens, pricedInputTokens+pricedOutputTokens, formatUSD(totalCost))
	} else {
		fmt.Fprintf(writer, "\tPRICED TOTAL\t\t%d\t%d\t%d\t\t\t%s\n", pricedInputTokens, pricedOutputTokens, pricedInputTokens+pricedOutputTokens, formatUSD(totalCost))
	}
	_ = writer.Flush()

	if unpricedInputTokens > 0 || unpricedOutputTokens > 0 {
		fmt.Printf("\nUnpriced tokens (--cost %s): input=%d output=%d total=%d\n", cost, unpricedInputTokens, unpricedOutputTokens, unpricedInputTokens+unpricedOutputTokens)
	}
}

func aggregateEventsWithCost(events []RequestEvent, period string, cost string) []CostAggregateRow {
	type key struct {
		period string
		model  string
	}

	rowsByKey := make(map[key]*CostAggregateRow)
	for _, event := range events {
		modelID := event.ModelID
		if modelID == "" {
			modelID = "unknown"
		}

		rowKey := key{
			period: periodLabel(event.TimestampMs, period),
			model:  modelID,
		}

		row := rowsByKey[rowKey]
		if row == nil {
			row = &CostAggregateRow{Period: rowKey.period, ModelID: rowKey.model}
			rowsByKey[rowKey] = row
		}

		row.Requests++
		row.PromptTokens += event.PromptTokens
		row.OutputTokens += event.OutputTokens

		price, ok := priceForModel(modelID, cost)
		if !ok {
			continue
		}
		inputCost, outputCost := price.costForUsage(event.PromptTokens, event.OutputTokens)
		row.Priced = true
		row.Price = price
		row.InputCost += inputCost
		row.OutputCost += outputCost
	}

	rows := make([]CostAggregateRow, 0, len(rowsByKey))
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

func uniqueModelsFromEvents(events []RequestEvent) []string {
	seen := make(map[string]bool)
	for _, event := range events {
		modelID := event.ModelID
		if modelID == "" {
			modelID = "unknown"
		}
		seen[modelID] = true
	}

	models := make([]string, 0, len(seen))
	for modelID := range seen {
		models = append(models, modelID)
	}
	sort.Strings(models)
	return models
}

func priceForModel(modelID string, cost string) (ModelPrice, bool) {
	for _, lookup := range []func(string) (ModelPrice, bool){
		anthropicPriceForModel,
		openAIPriceForModel,
		geminiPriceForModel,
	} {
		price, ok := lookup(modelID)
		if !ok {
			continue
		}
		if cost == "all" || strings.EqualFold(price.Provider, cost) {
			return price, true
		}
		return ModelPrice{}, false
	}

	return ModelPrice{}, false
}

func anthropicPriceForModel(modelID string) (ModelPrice, bool) {
	normalized := strings.ToLower(modelID)
	normalized = strings.ReplaceAll(normalized, "_", "-")

	switch {
	case strings.Contains(normalized, "claude-opus-4.7") || strings.Contains(normalized, "claude-opus-4-7"):
		return flatPrice("Anthropic", "Claude Opus 4.7", 5, 25), true
	case strings.Contains(normalized, "claude-opus-4.6") || strings.Contains(normalized, "claude-opus-4-6"):
		return flatPrice("Anthropic", "Claude Opus 4.6", 5, 25), true
	case strings.Contains(normalized, "claude-opus-4.5") || strings.Contains(normalized, "claude-opus-4-5"):
		return flatPrice("Anthropic", "Claude Opus 4.5", 5, 25), true
	case strings.Contains(normalized, "claude-opus-4.1") || strings.Contains(normalized, "claude-opus-4-1"):
		return flatPrice("Anthropic", "Claude Opus 4.1", 15, 75), true
	case strings.Contains(normalized, "claude-opus-4"):
		return flatPrice("Anthropic", "Claude Opus 4", 15, 75), true
	case strings.Contains(normalized, "claude-sonnet-4.6") || strings.Contains(normalized, "claude-sonnet-4-6"):
		return flatPrice("Anthropic", "Claude Sonnet 4.6", 3, 15), true
	case strings.Contains(normalized, "claude-sonnet-4.5") || strings.Contains(normalized, "claude-sonnet-4-5"):
		return flatPrice("Anthropic", "Claude Sonnet 4.5", 3, 15), true
	case strings.Contains(normalized, "claude-sonnet-4"):
		return flatPrice("Anthropic", "Claude Sonnet 4", 3, 15), true
	case strings.Contains(normalized, "claude-haiku-4.5") || strings.Contains(normalized, "claude-haiku-4-5"):
		return flatPrice("Anthropic", "Claude Haiku 4.5", 1, 5), true
	case strings.Contains(normalized, "claude-haiku-3.5") || strings.Contains(normalized, "claude-haiku-3-5"):
		return flatPrice("Anthropic", "Claude Haiku 3.5", 0.80, 4), true
	case strings.Contains(normalized, "claude-haiku-3"):
		return flatPrice("Anthropic", "Claude Haiku 3", 0.25, 1.25), true
	default:
		return ModelPrice{}, false
	}
}

func openAIPriceForModel(modelID string) (ModelPrice, bool) {
	normalized := normalizeModelForPricing(modelID)

	switch {
	case strings.Contains(normalized, "gpt-5.5-pro") || strings.Contains(normalized, "gpt-5-5-pro"):
		return flatPrice("OpenAI", "gpt-5.5-pro", 30, 180), true
	case strings.Contains(normalized, "gpt-5.5") || strings.Contains(normalized, "gpt-5-5"):
		return flatPrice("OpenAI", "gpt-5.5", 5, 30), true
	case strings.Contains(normalized, "gpt-5.4-pro") || strings.Contains(normalized, "gpt-5-4-pro"):
		return flatPrice("OpenAI", "gpt-5.4-pro", 30, 180), true
	case strings.Contains(normalized, "gpt-5.4-mini") || strings.Contains(normalized, "gpt-5-4-mini"):
		return flatPrice("OpenAI", "gpt-5.4-mini", 0.75, 4.50), true
	case strings.Contains(normalized, "gpt-5.4-nano") || strings.Contains(normalized, "gpt-5-4-nano"):
		return flatPrice("OpenAI", "gpt-5.4-nano", 0.20, 1.25), true
	case strings.Contains(normalized, "gpt-5.4") || strings.Contains(normalized, "gpt-5-4"):
		return flatPrice("OpenAI", "gpt-5.4", 2.50, 15), true
	case strings.Contains(normalized, "gpt-5.3-codex") || strings.Contains(normalized, "gpt-5-3-codex"):
		return flatPrice("OpenAI", "gpt-5.3-codex", 1.75, 14), true
	case strings.Contains(normalized, "gpt-5.3-chat-latest") || strings.Contains(normalized, "gpt-5-3-chat-latest"):
		return flatPrice("OpenAI", "gpt-5.3-chat-latest", 1.75, 14), true
	case strings.Contains(normalized, "gpt-4.1-nano") || strings.Contains(normalized, "gpt-4-1-nano"):
		return flatPrice("OpenAI", "gpt-4.1-nano", 0.10, 0.40), true
	case strings.Contains(normalized, "gpt-4.1-mini") || strings.Contains(normalized, "gpt-4-1-mini"):
		return flatPrice("OpenAI", "gpt-4.1-mini", 0.40, 1.60), true
	case strings.Contains(normalized, "gpt-4.1") || strings.Contains(normalized, "gpt-4-1"):
		return flatPrice("OpenAI", "gpt-4.1", 2, 8), true
	default:
		return ModelPrice{}, false
	}
}

func geminiPriceForModel(modelID string) (ModelPrice, bool) {
	normalized := normalizeModelForPricing(modelID)

	switch {
	case strings.Contains(normalized, "gemini-3.1-pro-preview") || strings.Contains(normalized, "gemini-3-1-pro-preview"):
		return tieredPrice("Gemini", "Gemini 3.1 Pro Preview", 2, 12, 200_000, 4, 18, "<=200k/>200k prompt-token tiers; output includes thinking tokens"), true
	case strings.Contains(normalized, "gemini-3-pro-preview") || strings.Contains(normalized, "gemini-3.0-pro-preview"):
		return tieredPrice("Gemini", "Gemini 3 Pro Preview text", 2, 12, 200_000, 4, 18, "uses current Gemini 3 Pro text tier rates; output includes thinking tokens"), true
	case strings.Contains(normalized, "gemini-3.1-flash-lite-preview") || strings.Contains(normalized, "gemini-3-1-flash-lite-preview"):
		return flatPriceWithNotes("Gemini", "Gemini 3.1 Flash-Lite Preview", 0.25, 1.50, "text/image/video input rate; audio input is higher; output includes thinking tokens"), true
	case strings.Contains(normalized, "gemini-3-flash-preview") || strings.Contains(normalized, "gemini-3.0-flash-preview"):
		return flatPriceWithNotes("Gemini", "Gemini 3 Flash Preview", 0.50, 3, "text/image/video input rate; audio input is higher; output includes thinking tokens"), true
	case strings.Contains(normalized, "gemini-2.5-computer-use-preview") || strings.Contains(normalized, "gemini-2-5-computer-use-preview"):
		return tieredPrice("Gemini", "Gemini 2.5 Computer Use Preview", 1.25, 10, 200_000, 2.50, 15, "<=200k/>200k prompt-token tiers"), true
	case strings.Contains(normalized, "gemini-2.5-pro") || strings.Contains(normalized, "gemini-2-5-pro"):
		return tieredPrice("Gemini", "Gemini 2.5 Pro", 1.25, 10, 200_000, 2.50, 15, "<=200k/>200k prompt-token tiers; output includes thinking tokens"), true
	case strings.Contains(normalized, "gemini-2.5-flash-lite") || strings.Contains(normalized, "gemini-2-5-flash-lite"):
		return flatPriceWithNotes("Gemini", "Gemini 2.5 Flash-Lite", 0.10, 0.40, "text/image/video input rate; audio input is higher; output includes thinking tokens"), true
	case strings.Contains(normalized, "gemini-2.5-flash") || strings.Contains(normalized, "gemini-2-5-flash"):
		return flatPriceWithNotes("Gemini", "Gemini 2.5 Flash", 0.30, 2.50, "text/image/video input rate; audio input is higher; output includes thinking tokens"), true
	case strings.Contains(normalized, "gemini-2.0-flash-lite") || strings.Contains(normalized, "gemini-2-0-flash-lite"):
		return flatPrice("Gemini", "Gemini 2.0 Flash-Lite", 0.075, 0.30), true
	case strings.Contains(normalized, "gemini-2.0-flash") || strings.Contains(normalized, "gemini-2-0-flash"):
		return flatPriceWithNotes("Gemini", "Gemini 2.0 Flash", 0.10, 0.40, "text/image/video input rate; audio input is higher; output includes thinking tokens"), true
	default:
		return ModelPrice{}, false
	}
}

func flatPrice(provider string, name string, inputPerMTok float64, outputPerMTok float64) ModelPrice {
	return flatPriceWithNotes(provider, name, inputPerMTok, outputPerMTok, "standard token rates")
}

func flatPriceWithNotes(provider string, name string, inputPerMTok float64, outputPerMTok float64, notes string) ModelPrice {
	return ModelPrice{
		Name:          name,
		Provider:      provider,
		InputPerMTok:  inputPerMTok,
		OutputPerMTok: outputPerMTok,
		Notes:         notes,
	}
}

func tieredPrice(provider string, name string, inputPerMTok float64, outputPerMTok float64, threshold int64, highInputPerMTok float64, highOutputPerMTok float64, notes string) ModelPrice {
	return ModelPrice{
		Name:                 name,
		Provider:             provider,
		InputPerMTok:         inputPerMTok,
		OutputPerMTok:        outputPerMTok,
		PromptTokenThreshold: threshold,
		HighInputPerMTok:     highInputPerMTok,
		HighOutputPerMTok:    highOutputPerMTok,
		Notes:                notes,
	}
}

func normalizeModelForPricing(modelID string) string {
	normalized := strings.ToLower(modelID)
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return normalized
}

func (price ModelPrice) costForUsage(inputTokens int64, outputTokens int64) (float64, float64) {
	inputRate, outputRate := price.ratesForPromptTokens(inputTokens)
	return costForTokens(inputTokens, inputRate), costForTokens(outputTokens, outputRate)
}

func (price ModelPrice) ratesForPromptTokens(inputTokens int64) (float64, float64) {
	if price.PromptTokenThreshold > 0 && inputTokens > price.PromptTokenThreshold {
		return price.HighInputPerMTok, price.HighOutputPerMTok
	}
	return price.InputPerMTok, price.OutputPerMTok
}

func (price ModelPrice) inputRateText() string {
	if price.PromptTokenThreshold > 0 {
		return fmt.Sprintf("%s/%s", formatRate(price.InputPerMTok), formatRate(price.HighInputPerMTok))
	}
	return formatRate(price.InputPerMTok)
}

func (price ModelPrice) outputRateText() string {
	if price.PromptTokenThreshold > 0 {
		return fmt.Sprintf("%s/%s", formatRate(price.OutputPerMTok), formatRate(price.HighOutputPerMTok))
	}
	return formatRate(price.OutputPerMTok)
}

func costForTokens(tokens int64, perMillionTokens float64) float64 {
	return float64(tokens) * perMillionTokens / 1_000_000
}

func formatUSD(value float64) string {
	return fmt.Sprintf("$%.2f", value)
}

func formatRate(value float64) string {
	return fmt.Sprintf("$%.2f", value)
}

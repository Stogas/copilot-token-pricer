package main

import "testing"

func TestPriceForModelMatchesOpenAIAndGemini(t *testing.T) {
	openAI, ok := priceForModel("copilot/gpt-5.3-codex", "all")
	if !ok {
		t.Fatal("expected gpt-5.3-codex to be priced")
	}
	if openAI.Provider != "OpenAI" || openAI.InputPerMTok != 1.75 || openAI.OutputPerMTok != 14 {
		t.Fatalf("unexpected OpenAI price: %+v", openAI)
	}

	gemini, ok := priceForModel("copilot/gemini-3.1-pro-preview", "all")
	if !ok {
		t.Fatal("expected gemini-3.1-pro-preview to be priced")
	}
	if gemini.Provider != "Gemini" || gemini.PromptTokenThreshold != 200_000 || gemini.HighInputPerMTok != 4 || gemini.HighOutputPerMTok != 18 {
		t.Fatalf("unexpected Gemini tiered price: %+v", gemini)
	}
}

func TestGeminiTieredCostIsCalculatedPerRequest(t *testing.T) {
	events := []RequestEvent{
		{ModelID: "copilot/gemini-3.1-pro-preview", PromptTokens: 100_000, OutputTokens: 1_000},
		{ModelID: "copilot/gemini-3.1-pro-preview", PromptTokens: 250_000, OutputTokens: 1_000},
	}

	rows := aggregateEventsWithCost(events, "none", "all")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	expectedInputCost := costForTokens(100_000, 2) + costForTokens(250_000, 4)
	expectedOutputCost := costForTokens(1_000, 12) + costForTokens(1_000, 18)
	if rows[0].InputCost != expectedInputCost || rows[0].OutputCost != expectedOutputCost {
		t.Fatalf("unexpected costs: got input=%f output=%f, want input=%f output=%f", rows[0].InputCost, rows[0].OutputCost, expectedInputCost, expectedOutputCost)
	}
}

func TestCostModeFiltersProviders(t *testing.T) {
	events := []RequestEvent{
		{ModelID: "copilot/gpt-5.3-codex", PromptTokens: 100_000, OutputTokens: 1_000},
	}

	rows := aggregateEventsWithCost(events, "none", "gemini")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Priced {
		t.Fatal("expected OpenAI model to be unpriced when --cost gemini")
	}
}

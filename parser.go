package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type SessionAnchor struct {
	ChatSessionID string
	CreationDate  int64
	ModelID       string
}

type RequestEvent struct {
	ChatSessionID string
	RequestIndex  int
	RequestID     string
	ModelID       string
	TimestampMs   int64
	PromptTokens  int64
	OutputTokens  int64
}

type ParsedFile struct {
	SourcePath    string
	WorkspaceID   string
	WorkspacePath string
	Anchor        SessionAnchor
	Requests      []RequestEvent
}

type parseState struct {
	anchor            SessionAnchor
	hasAnchor         bool
	requests          []RequestEvent
	requestModels     map[int]string
	requestIDs        map[int]string
	requestTimestamps map[int]int64
	nextRequestIndex  int
}

var trailingVersionDash = regexp.MustCompile(`^(.*-\d+)-(\d+)$`)

func parseJSONL(filePath, workspaceID, workspacePath string) (ParsedFile, error) {
	state := parseState{
		requestModels:     make(map[int]string),
		requestIDs:        make(map[int]string),
		requestTimestamps: make(map[int]int64),
	}

	file, err := os.Open(filePath)
	if err != nil {
		return finalizeParsedFile(filePath, workspaceID, workspacePath, state), err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal([]byte(raw), &obj); err != nil {
			continue
		}
		processLine(&state, obj)
	}

	return finalizeParsedFile(filePath, workspaceID, workspacePath, state), scanner.Err()
}

func processLine(state *parseState, obj map[string]any) {
	kind, ok := intValue(obj["kind"])
	if !ok {
		return
	}

	if kind == 0 {
		if v, ok := objectValue(obj["v"]); ok {
			handleSessionAnchor(state, v)
		} else {
			handleSessionAnchor(state, obj)
		}
		return
	}

	k, ok := arrayValue(obj["k"])
	if !ok {
		return
	}

	if kind == 2 && len(k) == 1 && stringValue(k[0]) == "requests" {
		if v, ok := arrayValue(obj["v"]); ok {
			handleNewRequests(state, v)
		}
		return
	}

	if kind == 1 && len(k) == 3 && stringValue(k[0]) == "requests" && stringValue(k[2]) == "result" {
		requestIndex, ok := intValue(k[1])
		if !ok {
			return
		}
		if v, ok := objectValue(obj["v"]); ok {
			handleResult(state, v, requestIndex)
		}
	}
}

func handleSessionAnchor(state *parseState, v map[string]any) {
	anchor := SessionAnchor{
		ChatSessionID: stringValue(v["sessionId"]),
		CreationDate:  int64Value(v["creationDate"]),
	}

	if inputState, ok := objectValue(v["inputState"]); ok {
		if selected, ok := objectValue(inputState["selectedModel"]); ok {
			anchor.ModelID = firstNonEmpty(
				stringValue(selected["identifier"]),
				stringValue(selected["id"]),
			)
		}
	}

	state.anchor = anchor
	state.hasAnchor = true
}

func handleNewRequests(state *parseState, items []any) {
	for _, item := range items {
		obj, ok := objectValue(item)
		if !ok {
			continue
		}

		idx := state.nextRequestIndex
		state.nextRequestIndex++

		if modelID := stringValue(obj["modelId"]); modelID != "" {
			state.requestModels[idx] = modelID
		}
		if requestID := stringValue(obj["requestId"]); requestID != "" {
			state.requestIDs[idx] = requestID
		}
		if timestamp := int64Value(obj["timestamp"]); timestamp != 0 {
			state.requestTimestamps[idx] = timestamp
		}
	}
}

func handleResult(state *parseState, v map[string]any, requestIndex int) {
	metadata, _ := objectValue(v["metadata"])
	usage, _ := objectValue(v["usage"])

	promptTokens := int64Value(metadata["promptTokens"])
	if promptTokens == 0 {
		promptTokens = int64Value(usage["promptTokens"])
	}

	outputTokens := int64Value(metadata["outputTokens"])
	if outputTokens == 0 {
		outputTokens = int64Value(usage["completionTokens"])
	}

	timestampMs := int64(0)
	if timings, ok := objectValue(v["timings"]); ok {
		timestampMs = firstNonZeroInt64(
			int64Value(timings["requestSent"]),
			int64Value(timings["firstTokenReceived"]),
		)
	}
	if timestampMs == 0 {
		if toolRounds, ok := arrayValue(metadata["toolCallRounds"]); ok && len(toolRounds) > 0 {
			if firstRound, ok := objectValue(toolRounds[0]); ok {
				timestampMs = int64Value(firstRound["timestamp"])
			}
		}
	}
	if timestampMs == 0 {
		timestampMs = state.requestTimestamps[requestIndex]
	}

	modelID := stringValue(metadata["modelId"])
	if modelID == "" {
		modelID = normalizeResolvedModel(stringValue(metadata["resolvedModel"]))
	}

	state.requests = append(state.requests, RequestEvent{
		ChatSessionID: state.anchor.ChatSessionID,
		RequestIndex:  requestIndex,
		RequestID:     state.requestIDs[requestIndex],
		ModelID:       modelID,
		TimestampMs:   timestampMs,
		PromptTokens:  promptTokens,
		OutputTokens:  outputTokens,
	})
}

func finalizeParsedFile(filePath, workspaceID, workspacePath string, state parseState) ParsedFile {
	stem := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	if !state.hasAnchor {
		state.anchor = SessionAnchor{ChatSessionID: stem}
	} else if state.anchor.ChatSessionID == "" {
		state.anchor.ChatSessionID = stem
	}

	for i := range state.requests {
		req := &state.requests[i]
		if req.ChatSessionID == "" {
			req.ChatSessionID = state.anchor.ChatSessionID
		}
		if req.ModelID == "" {
			req.ModelID = state.requestModels[req.RequestIndex]
		}
		if req.ModelID == "" {
			req.ModelID = state.anchor.ModelID
		}
		if req.ModelID == "" {
			req.ModelID = "unknown"
		}
		if req.RequestID == "" {
			req.RequestID = state.requestIDs[req.RequestIndex]
		}
		if req.TimestampMs == 0 {
			req.TimestampMs = state.anchor.CreationDate
		}
	}

	return ParsedFile{
		SourcePath:    filePath,
		WorkspaceID:   workspaceID,
		WorkspacePath: workspacePath,
		Anchor:        state.anchor,
		Requests:      state.requests,
	}
}

func normalizeResolvedModel(raw string) string {
	if raw == "" {
		return ""
	}
	return "copilot/" + trailingVersionDash.ReplaceAllString(raw, `$1.$2`)
}

func objectValue(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	return obj, ok
}

func arrayValue(value any) ([]any, bool) {
	array, ok := value.([]any)
	return array, ok
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func intValue(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case json.Number:
		i, err := strconv.Atoi(v.String())
		return i, err == nil
	default:
		return 0, false
	}
}

func int64Value(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		i, _ := strconv.ParseInt(v.String(), 10, 64)
		return i
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

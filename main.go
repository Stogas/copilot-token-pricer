package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr, commandName())
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "list":
		err = runList(os.Args[2:])
	case "report":
		err = runReport(os.Args[2:])
	case "help", "-h", "--help":
		printUsage(os.Stdout, commandName())
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runList(args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	storageRoot := flags.String("storage-root", "", "VS Code workspaceStorage directory")
	if err := flags.Parse(args); err != nil {
		return err
	}

	workspaces, err := discoverWorkspaces(*storageRoot)
	if err != nil {
		return fmt.Errorf("discover workspaces: %w", err)
	}
	if len(workspaces) == 0 {
		fmt.Println("No workspaces with chatSessions/*.jsonl found.")
		return nil
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tFILES\tPATH")
	for _, workspace := range workspaces {
		fmt.Fprintf(writer, "%s\t%d\t%s\n", workspace.ID, len(workspace.SessionFiles), displayWorkspacePath(workspace))
	}
	return writer.Flush()
}

func runReport(args []string) error {
	flags := flag.NewFlagSet("report", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	storageRoot := flags.String("storage-root", "", "VS Code workspaceStorage directory")
	workspaceQuery := flags.String("workspace", "", "workspace ID, exact path, or path substring")
	period := flags.String("period", "none", "time grouping: none, week, month")
	lastPeriods := flags.Int("last-periods", 0, "only include the current and previous N-1 periods for --period week or month")
	cost := flags.String("cost", "none", "cost model: none, anthropic, openai, gemini, all")
	all := flags.Bool("all", false, "aggregate all discovered workspaces")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *period != "none" && *period != "week" && *period != "month" {
		return errors.New("--period must be one of: none, week, month")
	}
	if *lastPeriods < 0 {
		return errors.New("--last-periods must be zero or greater")
	}
	if *lastPeriods > 0 && *period == "none" {
		return errors.New("--last-periods requires --period week or --period month")
	}
	if !validCostMode(*cost) {
		return errors.New("--cost must be one of: none, anthropic, openai, gemini, all")
	}
	if !*all && strings.TrimSpace(*workspaceQuery) == "" {
		return errors.New("pass --workspace <id-or-path>, or use --all")
	}

	workspaces, err := discoverWorkspaces(*storageRoot)
	if err != nil {
		return fmt.Errorf("discover workspaces: %w", err)
	}

	selected, err := selectWorkspaces(workspaces, *workspaceQuery, *all)
	if err != nil {
		return err
	}

	parsedFiles, events, err := parseSelectedWorkspaces(selected)
	if err != nil {
		return err
	}
	events = filterEventsByRecentPeriods(events, *period, *lastPeriods)
	rows := aggregateEvents(events, *period)

	if *cost != "none" {
		printPricingInformation(events, *cost)
	}
	printReportSummary(selected, parsedFiles, events)
	if *cost == "none" {
		printAggregateRows(rows, *period)
	} else {
		printCostRows(events, *period, *cost)
	}
	return nil
}

func validCostMode(cost string) bool {
	switch cost {
	case "none", "anthropic", "openai", "gemini", "all":
		return true
	default:
		return false
	}
}

func selectWorkspaces(workspaces []Workspace, query string, all bool) ([]Workspace, error) {
	if all {
		if len(workspaces) == 0 {
			return nil, errors.New("no workspaces with chatSessions/*.jsonl found")
		}
		return workspaces, nil
	}

	query = strings.TrimSpace(query)
	normalizedQuery := normalizePathForMatch(query)

	var exact []Workspace
	var partial []Workspace
	for _, workspace := range workspaces {
		if workspace.ID == query || normalizePathForMatch(workspace.Path) == normalizedQuery {
			exact = append(exact, workspace)
			continue
		}
		if workspace.Path != "" && strings.Contains(strings.ToLower(workspace.Path), strings.ToLower(query)) {
			partial = append(partial, workspace)
		}
	}

	matches := exact
	if len(matches) == 0 {
		matches = partial
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("workspace %q not found; run the list command to see choices", query)
	}
	if len(matches) > 1 {
		var labels []string
		for _, match := range matches {
			labels = append(labels, fmt.Sprintf("%s (%s)", match.ID, displayWorkspacePath(match)))
		}
		sort.Strings(labels)
		return nil, fmt.Errorf("workspace %q matched multiple workspaces: %s", query, strings.Join(labels, "; "))
	}
	return matches, nil
}

func parseSelectedWorkspaces(workspaces []Workspace) (int, []RequestEvent, error) {
	seen := make(map[string]RequestEvent)
	parsedFiles := 0

	for _, workspace := range workspaces {
		for _, filePath := range workspace.SessionFiles {
			parsed, err := parseJSONL(filePath, workspace.ID, workspace.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not fully parse %s: %v\n", filePath, err)
			}
			parsedFiles++
			for _, event := range parsed.Requests {
				key := fmt.Sprintf("%s:%d", event.ChatSessionID, event.RequestIndex)
				seen[key] = event
			}
		}
	}

	events := make([]RequestEvent, 0, len(seen))
	for _, event := range seen {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].TimestampMs != events[j].TimestampMs {
			return events[i].TimestampMs < events[j].TimestampMs
		}
		if events[i].ChatSessionID != events[j].ChatSessionID {
			return events[i].ChatSessionID < events[j].ChatSessionID
		}
		return events[i].RequestIndex < events[j].RequestIndex
	})

	return parsedFiles, events, nil
}

func printReportSummary(workspaces []Workspace, parsedFiles int, events []RequestEvent) {
	var promptTokens int64
	var outputTokens int64
	for _, event := range events {
		promptTokens += event.PromptTokens
		outputTokens += event.OutputTokens
	}

	fmt.Printf("Workspaces: %d\n", len(workspaces))
	if len(workspaces) == 1 {
		fmt.Printf("Workspace:  %s (%s)\n", displayWorkspacePath(workspaces[0]), workspaces[0].ID)
	}
	fmt.Printf("Files:      %d\n", parsedFiles)
	fmt.Printf("Requests:   %d\n", len(events))
	fmt.Printf("Input:      %d tokens\n", promptTokens)
	fmt.Printf("Output:     %d tokens\n", outputTokens)
	fmt.Printf("Total:      %d tokens\n\n", promptTokens+outputTokens)
}

func printAggregateRows(rows []AggregateRow, period string) {
	if len(rows) == 0 {
		fmt.Println("No completed request result events with token data found.")
		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if period == "none" {
		fmt.Fprintln(writer, "MODEL\tREQUESTS\tINPUT\tOUTPUT\tTOTAL")
		for _, row := range rows {
			fmt.Fprintf(writer, "%s\t%d\t%d\t%d\t%d\n", row.ModelID, row.Requests, row.PromptTokens, row.OutputTokens, row.PromptTokens+row.OutputTokens)
		}
	} else {
		fmt.Fprintln(writer, "PERIOD\tMODEL\tREQUESTS\tINPUT\tOUTPUT\tTOTAL")
		for _, row := range rows {
			fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%d\t%d\n", row.Period, row.ModelID, row.Requests, row.PromptTokens, row.OutputTokens, row.PromptTokens+row.OutputTokens)
		}
	}
	_ = writer.Flush()
}

func normalizePathForMatch(value string) string {
	if value == "" {
		return ""
	}
	cleaned := filepath.Clean(value)
	return strings.ToLower(cleaned)
}

func displayWorkspacePath(workspace Workspace) string {
	if workspace.Path != "" {
		return workspace.Path
	}
	return "(unknown)"
}

func commandName() string {
	name := filepath.Base(os.Args[0])
	if name == "" || name == "." {
		return "copilot-token-pricer"
	}
	return name
}

func printUsage(w io.Writer, command string) {
	fmt.Fprintln(w, "Copilot Token Pricer")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintf(w, "  %s list [--storage-root <path>]\n", command)
	fmt.Fprintf(w, "  %s report --workspace <id-or-path> [--period none|week|month] [--last-periods <n>] [--cost none|anthropic|openai|gemini|all] [--storage-root <path>]\n", command)
	fmt.Fprintf(w, "  %s report --all [--period none|week|month] [--last-periods <n>] [--cost none|anthropic|openai|gemini|all] [--storage-root <path>]\n", command)
}

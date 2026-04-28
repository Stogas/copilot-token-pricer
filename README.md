# Copilot Token Pricer

A small Go CLI for reading local VS Code Copilot chat session JSONL files and aggregating token usage by model, with optional estimated API pricing.

## Commands

```bash
# If running from source, replace `copilot-token-pricer` with `go run .`
copilot-token-pricer list

# Aggregate one workspace by model
copilot-token-pricer report --workspace <workspace-id-or-path>

# Aggregate one workspace by model and week/month
copilot-token-pricer report --workspace <workspace-id-or-path> --period week
copilot-token-pricer report --workspace <workspace-id-or-path> --period month

# Only include the current month, or the current and previous month
copilot-token-pricer report --workspace <workspace-id-or-path> --period month --last-periods 1
copilot-token-pricer report --workspace <workspace-id-or-path> --period month --last-periods 2

# Include estimated Anthropic API token costs
copilot-token-pricer report --workspace <workspace-id-or-path> --period month --cost anthropic

# Include estimated token costs for every known Anthropic, OpenAI, and Gemini model
copilot-token-pricer report --workspace <workspace-id-or-path> --period month --cost all

# Aggregate every discovered workspace
copilot-token-pricer report --all --period month
```

## Release builds

GitHub releases provide binaries for Linux, macOS, and Windows on both x86_64 and arm64. Only the macOS and Linux builds are currently tested by the maintainer.

Options:

- `--storage-root <path>` overrides VS Code's workspace storage root.
- `--workspace <id-or-path>` accepts the workspace storage ID, exact workspace path, or a case-insensitive substring of the workspace path.
- `--period none|week|month` controls time grouping. The default is `none`.
- `--last-periods <n>` limits `--period week` or `--period month` reports to the current period and previous `n-1` periods. For example, `--period month --last-periods 1` shows only the current month, while `--last-periods 2` shows the current and previous month. The default is `0`, which includes all periods.
- `--cost none|anthropic|openai|gemini|all` adds estimated token costs using public API input/output prices. The default is `none`. Cost reports print the matched pricing table before the usage summary so you can verify rates.

## Example output

The numbers below are sanitized sample data and are not intended to represent real usage.

```text
$ copilot-token-pricer report --all --period month --cost all --last-periods 1
Pricing model: all
Pricing basis: standard API token prices per 1M tokens; excludes cache discounts, batch/flex/priority modes, data residency uplifts, tool/search charges, media-specific charges, and Copilot subscription effects.
Pricing sources: Anthropic configured public API rates; OpenAI API pricing https://developers.openai.com/api/docs/pricing and https://openai.com/api/pricing/; Gemini Developer API pricing https://ai.google.dev/gemini-api/docs/pricing (last updated 2026-04-22 UTC).
Tiered Gemini Pro prices are calculated per request using that request's input tokens.
DETECTED MODEL                     PROVIDER   PRICING MODEL          IN$/MTOK  OUT$/MTOK  NOTES
copilot/claude-haiku-4-5.20251001  Anthropic  Claude Haiku 4.5       $1.00     $5.00      standard token rates
copilot/claude-opus-4.6            Anthropic  Claude Opus 4.6        $5.00     $25.00     standard token rates
copilot/claude-sonnet-4.6          Anthropic  Claude Sonnet 4.6      $3.00     $15.00     standard token rates
copilot/gemini-3-flash-preview     Gemini     Gemini 3 Flash Preview $0.50     $3.00      text/image/video input rate; audio input is higher; output includes thinking tokens
copilot/gpt-5.3-codex              OpenAI     gpt-5.3-codex          $1.75     $14.00     standard token rates
copilot/gpt-5.5                    OpenAI     gpt-5.5                $5.00     $30.00     standard token rates

Workspaces: 18
Files:      96
Requests:   214
Input:      12874620 tokens
Output:     177835 tokens
Total:      13052455 tokens

PERIOD   MODEL                              REQUESTS  INPUT     OUTPUT  TOTAL     IN$/MTOK  OUT$/MTOK  COST
2026-04  copilot/claude-sonnet-4.6          146       8923400   116900  9040300   $3.00     $15.00     $28.52
2026-04  copilot/claude-opus-4.6            31        2415600   42780   2458380   $5.00     $25.00     $13.15
2026-04  copilot/claude-haiku-4-5.20251001  18        728900    8300    737200    $1.00     $5.00      $0.77
2026-04  copilot/gemini-3-flash-preview     11        806720    9855    816575    $0.50     $3.00      $0.43
2026-04  copilot/gpt-5.3-codex              6         0         0       0         $1.75     $14.00     $0.00
2026-04  copilot/gpt-5.5                    2         0         0       0         $5.00     $30.00     $0.00
         PRICED TOTAL                                 12874620  177835  13052455                       $42.87
```

## Misc

Default VS Code workspace storage roots:

- Windows: `%APPDATA%\\Code\\User\\workspaceStorage`
- macOS: `~/Library/Application Support/Code/User/workspaceStorage`
- Linux: `${XDG_CONFIG_HOME:-~/.config}/Code/User/workspaceStorage`

The parser focuses on modern `chatSessions/*.jsonl` files. It extracts request results from JSONL `kind=1` entries, request metadata from `kind=2` entries, and session metadata from `kind=0` entries.

The tool reads local VS Code storage files only and does not send data to any external service. Report output may include local workspace paths and aggregated model/token usage, so review output before sharing it.

Cost estimates use standard API input and output token prices only. They do not account for prompt cache writes, cache hits, batch/flex/priority discounts or premiums, data residency multipliers, fast mode, media-specific charges, Copilot subscription effects, or non-token tool/search charges. Gemini Pro tiered pricing is calculated per request using that request's input token count.

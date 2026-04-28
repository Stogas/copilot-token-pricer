# Copilot Token Pricer

A small Go CLI for reading local VS Code Copilot chat session JSONL files and aggregating token usage by model, with optional estimated API pricing.

## Commands

```bash
# From this directory
go run . list

# Aggregate one workspace by model
go run . report --workspace <workspace-id-or-path>

# Aggregate one workspace by model and week/month
go run . report --workspace <workspace-id-or-path> --period week
go run . report --workspace <workspace-id-or-path> --period month

# Include estimated Anthropic API token costs
go run . report --workspace <workspace-id-or-path> --period month --cost anthropic

# Include estimated token costs for every known Anthropic, OpenAI, and Gemini model
go run . report --workspace <workspace-id-or-path> --period month --cost all

# Aggregate every discovered workspace
go run . report --all --period month
```

## Release builds

GitHub releases provide binaries for Linux, macOS, and Windows on both x86_64 and arm64. Only the macOS and Linux builds are currently tested by the maintainer.

Options:

- `--storage-root <path>` overrides VS Code's workspace storage root.
- `--workspace <id-or-path>` accepts the workspace storage ID, exact workspace path, or a case-insensitive substring of the workspace path.
- `--period none|week|month` controls time grouping. The default is `none`.
- `--cost none|anthropic|openai|gemini|all` adds estimated token costs using public API input/output prices. The default is `none`. Cost reports print the matched pricing table before the usage summary so you can verify rates.

Default VS Code workspace storage roots:

- Windows: `%APPDATA%\\Code\\User\\workspaceStorage`
- macOS: `~/Library/Application Support/Code/User/workspaceStorage`
- Linux: `${XDG_CONFIG_HOME:-~/.config}/Code/User/workspaceStorage`

The parser focuses on modern `chatSessions/*.jsonl` files. It extracts request results from JSONL `kind=1` entries, request metadata from `kind=2` entries, and session metadata from `kind=0` entries.

The tool reads local VS Code storage files only and does not send data to any external service. Report output may include local workspace paths and aggregated model/token usage, so review output before sharing it.

Cost estimates use standard API input and output token prices only. They do not account for prompt cache writes, cache hits, batch/flex/priority discounts or premiums, data residency multipliers, fast mode, media-specific charges, Copilot subscription effects, or non-token tool/search charges. Gemini Pro tiered pricing is calculated per request using that request's input token count.

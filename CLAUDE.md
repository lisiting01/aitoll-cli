# aitoll-cli Project Memory

## Project Overview
`aitoll-cli` — Go CLI for the AI-Toll platform (aitoll.net). Uses Cobra framework.

**Main repo:** `C:\CodingProject\ai-toll` (Next.js platform)
**CLI repo:** `C:\CodingProject\ai-toll-cli`

## Current State (v0.1.0 + passcard + workdir line pinning)

### Commands implemented
- `aitoll login/logout/status` — JWT auth via browser OAuth
- `aitoll passcard list` — list user's passcards (uses JWT)
- `aitoll passcard routes` — show services + lines for current passcard
- `aitoll passcard switch <service-name> <line-name>` — passcard-level line switch (case-insensitive), `auto` resets to automatic
- `aitoll line set/unset/reset/show` — per-working-directory line pinning via Claude Code's `ANTHROPIC_CUSTOM_HEADERS` setting

### Key architecture
- Auth: JWT stored at `{UserConfigDir}/aitoll/credentials.json`
- Passcard auto-detect: reads `~/.claude/settings.json` → `env.ANTHROPIC_AUTH_TOKEN` (pass_xxx key written by cc-switch or similar tools)
- Resolves pass_xxx → MongoDB ID via `GET /api/passes/current` (Bearer pass_xxx)
- Name→ID resolution: CLI calls `GET /api/passes/{id}/preference`, matches names client-side
- **Workdir pinning**: CLI writes `x-aitoll-line-<serviceId>: <lineId>` (plus optional `x-aitoll-line-mode: loose`) into `.claude/settings.local.json` or `.claude/settings.json` under `env.ANTHROPIC_CUSTOM_HEADERS`. Claude Code injects these into every API request at startup. Platform `resolveServiceLine()` consumes them at priority 0 (above passcard preference). Gateway strips all `x-aitoll-*` headers before forwarding to upstream.

### Platform API endpoints used
| Endpoint | Auth | Purpose |
|---|---|---|
| `GET /api/user/passes` | JWT Bearer | list passcards |
| `GET /api/passes/current` | Bearer pass_xxx | resolve API key → passcard ID |
| `GET /api/passes/{id}/preference` | JWT Bearer | get services + lines |
| `PATCH /api/passes/{id}/preference` | JWT Bearer | switch line (passcard-level) |

`aitoll line set/unset/reset/show` write **locally** to `.claude/settings*.json` in the current working directory — no platform endpoint needed. The platform reads the header at request time.

### Key files
- `cmd/passcard.go` — all passcard subcommands
- `cmd/line.go` — workdir line pinning subcommands
- `internal/api/client.go` — all API structs + methods
- `internal/claudesettings/settings.go` — read/write of `.claude/settings*.json` `env.ANTHROPIC_CUSTOM_HEADERS`
- `internal/config/config.go` — `ClaudeSettingsPath()` for the user-level settings file
- Platform: `C:\CodingProject\ai-toll\lib\proxy-utils.ts` (`resolveServiceLine` priority 0 + `WorkdirPinUnavailableError`)
- Platform: `C:\CodingProject\ai-toll\lib\gateway-utils.ts` (`AITOLL_HEADER_PREFIX`, `extractWorkdirPin`)
- Platform: 4 gateway routes under `C:\CodingProject\ai-toll\app\api\gateway\{cli,api,cherry,bot}\[...path]\route.ts` — header extraction, 503 mapping, upstream header stripping

### Workdir pin header protocol
- `x-aitoll-line-<serviceId>: <lineId>` — both are MongoDB ObjectIds; CLI resolves names → IDs before writing
- `x-aitoll-line-mode: loose` — opt-in to silent fallback if the pinned line is unavailable; default is strict (request fails with 503)
- All `x-aitoll-*` headers are stripped from the request before it's forwarded upstream

## Code Patterns
- Commands: `cmd/xxx.go`, register via `init()` + `rootCmd.AddCommand()`
- API methods: take `token string`, use `Authorization: Bearer`, return `{success, data, error}` JSON
- Output: plain `fmt.Fprintf(cmd.OutOrStdout(), ...)`, no table libraries
- No new dependencies — only `github.com/spf13/cobra`

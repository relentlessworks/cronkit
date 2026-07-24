# cronkit

> Agentic-first scheduled jobs and cron service. Plain text API, agent-driven, single Go binary.

The agent IS the interface. No UI, no SDK. The AI agent (ChatGPT, Claude, Cursor, etc.) is the user. The API is the product.

## Quick Start

```bash
# Build and run
make build
./cronkit

# Or with Go directly
go run ./cmd/cronkit

# It listens on :7777 by default
curl http://localhost:7777/help
```

## Auth Flow

```bash
# 1. Request OTP
curl -X POST http://localhost:7777/auth/request -d 'email=agent@example.com'

# 2. Verify OTP (code is logged to stderr in dev mode)
curl -X POST http://localhost:7777/auth/verify -d 'email=agent@example.com&code=123456'
# Returns: token=<bearer-token> workspace=<ws-handle>

# 3. Use the token for all subsequent requests
curl -H "Authorization: Bearer <token>" http://localhost:7777/jobs
```

## Creating Jobs

```bash
# Create a job with cron schedule
curl -X POST http://localhost:7777/jobs \
  -H "Authorization: Bearer <token>" \
  -d 'name=daily-report&schedule=0 9 * * *&url=https://api.example.com/report&method=POST'

# Create a job with interval schedule
curl -X POST http://localhost:7777/jobs \
  -H "Authorization: Bearer <token>" \
  -d 'name=health-check&schedule=5m&url=https://api.example.com/health&method=GET'

# List all jobs
curl http://localhost:7777/jobs -H "Authorization: Bearer <token>"

# Get a specific job
curl http://localhost:7777/jobs/job_abc12 -H "Authorization: Bearer <token>"

# Update a job
curl -X PATCH http://localhost:7777/jobs/job_abc12 \
  -H "Authorization: Bearer <token>" \
  -d 'enabled=false'

# Delete a job
curl -X DELETE http://localhost:7777/jobs/job_abc12 -H "Authorization: Bearer <token>"

# Manually trigger a job
curl -X POST http://localhost:7777/jobs/job_abc12/trigger -H "Authorization: Bearer <token>"
```

## Run Logs

```bash
# List recent runs
curl http://localhost:7777/runs -H "Authorization: Bearer <token>"

# List runs for a specific job
curl http://localhost:7777/runs/job_abc12 -H "Authorization: Bearer <token>"
```

## Schedule Formats

### Cron Expressions (5-field)
```
*/5 * * * *      Every 5 minutes
0 * * * *        Every hour at minute 0
0 9 * * 1-5      9am on weekdays
0 0 * * 0        Midnight on Sundays
0,30 * * * *     Every 30 minutes
0-59/15 * * * *  Every 15 minutes
```

### Intervals
```
30s              Every 30 seconds
5m               Every 5 minutes
1h               Every hour
2h30m            Every 2 hours 30 minutes
```

## Response Format

Plain text by default — one labeled, grepable line per record:
```
handle=job_k7m2q name=daily-report schedule=0 9 * * * type=cron url=https://api.example.com/report method=POST enabled=true next_run=2026-07-24T09:00:00Z last_run=never runs=0
```

JSON available via `Accept: application/json` header or `?format=json` query param.

Errors include a hint for self-correction:
```
error: schedule is required | hint: send schedule=<cron-expr-or-interval> e.g. schedule=*/5 * * * * or schedule=5m
```

## Configuration

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `-addr` | `CRONKIT_ADDR` | `:7777` | Listen address |
| `-data` | `CRONKIT_DATA_DIR` | `./data` | Data directory |
| `-secret` | `CRONKIT_SECRET` | (auto) | Token signing secret |
| `-smtp-host` | `CRONKIT_SMTP_HOST` | (empty) | SMTP server host |
| `-smtp-port` | `CRONKIT_SMTP_PORT` | `587` | SMTP server port |
| `-smtp-user` | `CRONKIT_SMTP_USER` | (empty) | SMTP username |
| `-smtp-pass` | `CRONKIT_SMTP_PASS` | (empty) | SMTP password |
| `-from-email` | `CRONKIT_FROM_EMAIL` | `noreply@cronkit.local` | From email address |

Config priority: defaults < env vars < flags.

When SMTP is not configured, OTP codes are logged to stderr for development.

## Build

```bash
make build    # CGO_ENABLED=0, single static binary
make test     # go test -race
make vet      # go vet
make run      # build and run
make clean    # remove binary and data
```

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| GET | `/help` | API documentation (also at `/.well-known/agent.md`) |
| POST | `/auth/request` | Request OTP code |
| POST | `/auth/verify` | Verify OTP, get bearer token |
| POST | `/workspaces` | Create a workspace |
| GET | `/workspaces` | List workspaces |
| POST | `/jobs` | Create a scheduled job |
| GET | `/jobs` | List all jobs |
| GET | `/jobs/<handle>` | Get a specific job |
| PATCH | `/jobs/<handle>` | Update a job |
| DELETE | `/jobs/<handle>` | Delete a job |
| POST | `/jobs/<handle>/trigger` | Manually trigger a job |
| GET | `/runs` | List recent run logs |
| GET | `/runs/<job-handle>` | List runs for a specific job |
| GET | `/health` | Health check |

## License

MIT

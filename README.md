# Vigil

A vigilant log monitor that watches your services and automatically creates deduplicated issues in Gitea when errors occur.

## Features

- Polls Loki for error logs (status >= 500 or level = ERROR)
- Auto-generates unique bug IDs for deduplication
- Creates issues in Gitea with full error details
- Adds comments to existing issues for duplicate occurrences
- Reopens closed issues if the error recurs
- Optional notifications to Slack, Discord, and Telegram

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────┐
│  Your Go    │────▶│  Promtail   │────▶│     Loki        │
│  Backend    │     │             │     │                 │
└─────────────┘     └─────────────┘     └────────┬────────┘
                                                 │
                                                 ▼
┌─────────────────────────────────────────────────────────┐
│                       Vigil                             │
│  - Polls Loki API every 30 seconds                      │
│  - Generates bugId for deduplication                    │
│  - Creates/updates issues in Gitea                      │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                       Gitea                             │
│  - Web UI for viewing/managing issues                   │
│  - Labels for filtering (severity, bugid)               │
│  - Comments track occurrence history                    │
└─────────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Start the services

```bash
cp .env.example .env
docker-compose up -d
```

### 2. Configure Gitea (first time only)

1. Open http://localhost:3000
2. Complete the setup wizard:
   - Database: SQLite (pre-configured)
   - Site Title: "Error Tracker" (or your preference)
   - Create an admin account

3. Create a repository for issues:
   - Click "+" → "New Repository"
   - Name: `error-issues`
   - Initialize with README (optional)

4. Generate an API token:
   - Go to Settings → Applications → Generate New Token
   - Name: "issue-tracker"
   - Select all permissions under "repository"
   - Copy the token

5. Update your `.env` file:
   ```bash
   GITEA_TOKEN=your_token_here
   GITEA_OWNER=your_username
   GITEA_REPO=error-issues
   ```

6. Restart Vigil:
   ```bash
   docker-compose restart vigil
   ```

### 3. Connect to your Loki instance

Update `LOKI_URL` in your `.env` or `docker-compose.yml` to point to your Loki instance.

For integration with an existing monitoring stack, add Vigil to your network:

```yaml
# In your existing docker-compose.yml
services:
  vigil:
    image: vigil:latest  # or build from source
    environment:
      - LOKI_URL=http://loki:3100
      - GITEA_URL=http://gitea:3000
      - GITEA_TOKEN=${GITEA_TOKEN}
      - GITEA_OWNER=${GITEA_OWNER}
      - GITEA_REPO=error-issues
    networks:
      - your-monitoring-network
```

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `LOKI_URL` | Yes | `http://loki:3100` | Loki server URL |
| `LOKI_POLL_INTERVAL` | No | `30s` | How often to poll Loki |
| `LOKI_LOOKBACK` | No | `5m` | Initial lookback period |
| `GITEA_URL` | Yes | - | Gitea server URL |
| `GITEA_TOKEN` | Yes | - | Gitea API access token |
| `GITEA_OWNER` | Yes | - | Repository owner (user/org) |
| `GITEA_REPO` | No | `error-issues` | Repository name |
| `SLACK_WEBHOOK_URL` | No | - | Slack webhook for notifications |
| `DISCORD_WEBHOOK_URL` | No | - | Discord webhook for notifications |
| `TELEGRAM_BOT_TOKEN` | No | - | Telegram bot token |
| `TELEGRAM_CHAT_ID` | No | - | Telegram chat ID |

## Issue Format

### Title
```
[500] PUT /api/v1/coffee/:id - Database timeout
```

### Body (Markdown)
```markdown
## Error Details

**Message:** Database connection timeout
**Source:** `ljos.app/brew/server.UpdateCoffee`
**File:** `/app/server/coffee_handler.go:142`

## Request Info

- **Method:** PUT
- **Endpoint:** /api/v1/log/coffee/287
- **Status Code:** 500
- **Request ID:** `6fe6a405-a8cf-482e-8c4d-963eaa61c458`

## Sample Log

```json
{ ... }
```
```

### Labels
- `auto-generated` - Marks automatically created issues
- `bugid:abc12345` - Unique ID for deduplication
- `severity:critical` - For 500 errors
- `severity:error` - For ERROR level logs

## Deduplication

Issues are deduplicated using a `bugId` which is:

1. **Explicit** (if provided in your logs):
   ```go
   logger.Error("Database timeout", "bugId", "db-timeout-coffee")
   ```

2. **Auto-generated** from:
   - HTTP method
   - Normalized endpoint (IDs replaced with `:id`)
   - Status code
   - Source function

Example: All `PUT /api/v1/coffee/123` and `PUT /api/v1/coffee/456` errors will share the same issue.

## Workflow

1. **New error occurs** → Issue created in Gitea with full details
2. **Same error recurs** → Comment added to existing issue
3. **Closed issue error recurs** → Issue reopened automatically
4. **Fix deployed** → Close the issue in Gitea UI
5. **Error recurs after fix** → Issue reopened (regression detected)

## Gitea Setup (Standalone)

If you prefer to run Gitea separately:

### Docker

```bash
docker run -d \
  --name gitea \
  -p 3000:3000 \
  -v gitea-data:/data \
  gitea/gitea:1.21
```

### Binary

```bash
# Download
wget -O gitea https://dl.gitea.io/gitea/1.21.0/gitea-1.21.0-linux-amd64
chmod +x gitea

# Run
./gitea web
```

## Building

```bash
# Build binary
go build -o vigil .

# Build Docker image
docker build -t vigil .
```

## Project Structure

```
vigil/
├── main.go              # Entry point
├── gitea/
│   └── client.go        # Gitea API client
├── loki/
│   └── client.go        # Loki API client
├── processor/
│   └── processor.go     # Log processing & deduplication
├── notifier/
│   ├── notifier.go      # Notifier interface
│   ├── slack.go         # Slack webhook
│   ├── discord.go       # Discord webhook
│   └── telegram.go      # Telegram bot
├── Dockerfile
├── docker-compose.yml
└── .env.example
```

## License

MIT

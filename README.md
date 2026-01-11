# Postal Inspection Service

A personal email management tool for iCloud that I vibe coded and am putting out there in case it's useful to anyone
else.

**Disclaimer**: This was built for my own use. It works for me, but your mileage may vary. No warranties, no promises,
just something that scratches my own itch.

## What It Does

Monitors your iCloud mailbox and automatically processes emails based on rules you set by moving emails into special
folders:

- **Block senders**: Move an email to `USPIS/Block` and that sender gets blocked. All their emails get deleted.
- **Transactional only**: Move an email to `USPIS/Transactional Only` and you'll only receive important emails (order
  confirmations, shipping updates, receipts) from that sender. Marketing emails get filtered out.

There's a simple web dashboard to view your blocked senders, transactional-only senders, and an action log of everything
the service has done.

## How It Works

1. You move an unwanted email to one of the USPIS folders
2. The service polls your mailbox (default: every minute)
3. It processes the email, adds the sender to the appropriate list
4. Going forward, emails from that sender are handled automatically
5. The web dashboard shows you what's happening

The classifier distinguishes transactional emails (receipts, shipping notifications, password resets) from marketing
emails (sales, newsletters, promotions). When in doubt, it assumes marketing.

## Requirements

- An iCloud email account
- An app-specific password (generate one at https://appleid.apple.com under Sign-In and Security > App-Specific
  Passwords)
- Docker (or Go 1.23+ if running directly)

## Setup

1. Copy the example environment file:
   ```
   cp .env.example .env
   ```

2. Edit `.env` with your credentials:
   ```
   ICLOUD_EMAIL=your-email@icloud.com
   ICLOUD_APP_PASSWORD=your-app-specific-password
   ```

3. Run with Docker Compose:
   ```
   docker-compose up -d
   ```

4. Access the dashboard at http://localhost:8080

The service will create the `USPIS/Block` and `USPIS/Transactional Only` folders in your iCloud mailbox automatically.

## Configuration

| Variable              | Default           | Description                       |
|-----------------------|-------------------|-----------------------------------|
| `ICLOUD_EMAIL`        | (required)        | Your iCloud email address         |
| `ICLOUD_APP_PASSWORD` | (required)        | App-specific password             |
| `POLL_INTERVAL`       | `1m`              | How often to check for new emails |
| `WEB_PORT`            | `8080`            | Port for the web dashboard        |
| `DB_PATH`             | `/data/postal.db` | SQLite database path              |

## Diagnostics

There's a diagnostic tool to inspect your USPIS folders:

```
go run cmd/diagnose/main.go
```

This shows folder statistics and lists messages, useful for troubleshooting.

## Project Structure

```
cmd/
  server/       - Main application
  diagnose/     - Diagnostic utility
internal/
  classifier/   - Email classification (transactional vs marketing)
  config/       - Configuration loading
  db/           - SQLite database operations
  imap/         - IMAP client for iCloud
  poller/       - Background polling and processing
  web/          - Web dashboard
```

## License

MIT - do whatever you want with it.
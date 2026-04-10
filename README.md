# kbchat

A Go CLI for exporting and reading Keybase chat history offline. Exports DMs, group chats, and team channels to a local directory tree, then lets you list, view, and search conversations without a running Keybase client.

Replaces the TypeScript/Deno [keybase-export](https://github.com/eilvelia/keybase-export) with a native Go implementation that uses the Keybase chat API directly via `keybase chat api`.

## Features

- Per-message directory structure (`messages/<id>/message.json`) for O(1) lookups
- Content-addressable attachment storage (`<sha256>.<ext>`)
- Full history export via prev-chain crawling (bypasses the ~1000 message pagination limit)
- Incremental backups with automatic gap detection and backfill
- Concurrent downloads via goroutine worker pool
- Preserves full message history (no edit/delete collapsing)
- Offline list, view, and grep — no Keybase client needed for read commands

## Usage

| Command | Description |
|---------|-------------|
| `export` | Export Keybase chat history to local store |
| `list` (alias `ls`) | List conversations in the local store |
| `view` | Display messages from one or more conversations |
| `grep` | Search messages across conversations |
| `help` | Show usage information |

Conversation arguments support glob patterns (`*`, `**`, `?`) matched against the store path. Plain strings also match as prefixes (e.g. `Team/engineering` matches `Team/engineering/general`).

```sh
# Export everything
kbchat export ~/keybase-backup

# Export specific conversations
kbchat export ~/keybase-backup Chat/alice Team/engineering

# Continuous export every 10 minutes with log file
kbchat export --continuous --interval=10m --log-file=/var/log/kbchat.log

# List conversations (long format shows type, count, size, timestamps)
kbchat list -l

# View last 20 messages from a DM
kbchat view Chat/alice

# View messages from a time range
kbchat view --after '3 days ago' --before yesterday Chat/alice

# Search across all conversations (pattern is a Go regexp)
kbchat grep 'deploy'

# Regex search with context in a specific team
kbchat grep -C 3 Team/engineering 'error|fail'
```

Run `kbchat help <command>` for subcommand-specific usage and options.

## Install

Requires Go 1.23+ and [Keybase](https://keybase.io) CLI installed and authenticated (for `export` only).

```sh
go install github.com/major0/kbchat@latest
```

## Configuration

Create `~/.config/kbchat/config.json`:

```json
{
    "store_path": "/home/user/keybase-backup",
    "time_format": "2006-01-02 15:04:05.00 MST"
}
```

| Field | Description | Required |
|-------|-------------|----------|
| `store_path` | Root directory of exported chat data | yes |
| `time_format` | Go time layout for display timestamps | no (default: `2006-01-02 15:04:05.00 MST`) |

## Development

```sh
make test              # Run tests
make coverage          # Run tests with coverage
make coverage-validate # Validate coverage meets 70% floor
make lint              # Run linter
make static-check      # Full static analysis
```

Pre-commit hooks:

```sh
pre-commit install
pre-commit install --hook-type commit-msg
```

## License

MIT License — Copyright (c) 2026 Mark Ferrell. See [LICENSE](LICENSE).

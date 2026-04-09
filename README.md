# kbchat

A Go CLI for exporting and reading Keybase chat history offline. Exports DMs, group chats, and team channels to a local directory tree, then lets you list, view, and search conversations without a running Keybase client.

Replaces the TypeScript/Deno [keybase-export](https://github.com/eilvelia/keybase-export) with a native Go implementation that uses the Keybase chat API directly via `keybase chat api`.

## Features

- Per-message directory structure (`messages/<id>/message.json`) for O(1) lookups
- Content-addressable attachment storage (`<sha256>.<ext>`)
- Incremental backups via message-chain walking
- Concurrent downloads via goroutine worker pool
- Preserves full message history (no edit/delete collapsing)
- Offline list, view, and search — no Keybase client needed for read commands

## Requirements

- Go 1.23+
- [Keybase](https://keybase.io) CLI installed and authenticated (for `export` only)

## Install

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

## Subcommands

| Command | Description |
|---------|-------------|
| `export` | Export Keybase chat history to local store |
| `list` (alias `ls`) | List conversations in the local store |
| `view` | Display messages from a single conversation |
| `search` | Search messages across conversations |
| `help` | Show usage information |

## Usage

### export

Export Keybase chat history to a local directory.

```sh
kbchat export [options] [destdir] [filters...]
```

When `destdir` is omitted, uses `store_path` from the config file.

Options:

| Flag | Description | Default |
|------|-------------|---------|
| `-P`, `--parallel=<n>` | Concurrent workers | 4 |
| `--verbose` | Detailed logging | off |
| `--skip-attachments` | Skip attachment downloads | off |
| `--continuous` | Run in a loop | off |
| `--interval=<duration>` | Sleep between cycles (with `--continuous`) | 5m |
| `--log-file=<path>` | Redirect logs to file (append mode, SIGHUP reopens) | — |

Examples:

```sh
# Export everything
kbchat export ~/keybase-backup

# Export specific conversations
kbchat export ~/keybase-backup Chat/alice Team/engineering

# Export without attachments, 8 workers
kbchat export -P 8 --skip-attachments ~/keybase-backup

# Continuous export every 10 minutes
kbchat export --continuous --interval=10m

# Export with log file (supports logrotate via SIGHUP)
kbchat export --continuous --log-file=/var/log/kbchat.log
```

### list

List conversations in the local store.

```sh
kbchat list [options] [patterns...]
```

Options:

| Flag | Description |
|------|-------------|
| `-1` | One conversation per line (default when stdout is not a terminal) |
| `-C` | Column format (default when stdout is a terminal) |
| `-l`, `--verbose` | Long format: type, message count, timestamps, name |
| `--format=<fmt>` | Named format (`single-column`, `columns`, `long`) or custom format string |

Examples:

```sh
# List all conversations
kbchat list

# List with long format
kbchat list -l

# Filter by pattern
kbchat list 'Chats/*bob*' 'Teams/engineering/*'
```

### view

Display messages from a single conversation.

```sh
kbchat view [options] <filter>
```

Options:

| Flag | Description | Default |
|------|-------------|---------|
| `--count=<n>` | Number of messages (`0` for all) | 20 |
| `--date=<YYYY-MM-DD>` | Show messages from a specific day | — |
| `--after=<timestamp>` | Show messages after timestamp | — |
| `--before=<timestamp>` | Show messages before timestamp | — |
| `--verbose` | Include message IDs and metadata | off |

Examples:

```sh
# View last 20 messages from a DM
kbchat view Chat/alice

# View all messages from a team channel
kbchat view --count 0 Team/engineering/general

# View messages from a specific day
kbchat view --date 2025-01-15 Chat/bob
```

### search

Search messages across conversations.

```sh
kbchat search [options] <pattern> [filters...]
```

Options:

| Flag | Description |
|------|-------------|
| `-G`, `--regexp` | Basic regular expression |
| `-E`, `--enhanced-regexp` | Extended regular expression |
| `-P`, `--pcre` | PCRE-compatible pattern |
| `-i` | Case-insensitive matching |
| `-A <n>` | Show n messages after each match |
| `-B <n>` | Show n messages before each match |
| `-C <n>` | Show n messages before and after |
| `--after=<timestamp>` | Search messages after timestamp |
| `--before=<timestamp>` | Search messages before timestamp |
| `--count=<n>` | Limit total results |
| `--verbose` | Include message IDs and conversation IDs |

Examples:

```sh
# Glob search across all conversations
kbchat search '*deploy*'

# Regex search in a specific team
kbchat search -E 'error|fail' Team/engineering

# Search with context
kbchat search -C 3 'outage' Team/ops

# Search within a time range
kbchat search --after '3 days ago' --before yesterday 'release'
```

## Output Structure

```
<store_path>/
├── Chats/
│   ├── alice/
│   │   ├── messages/
│   │   │   ├── 1/
│   │   │   │   └── message.json
│   │   │   └── 2/
│   │   │       ├── message.json
│   │   │       └── attachments.json
│   │   ├── attachments/
│   │   │   └── <sha256>.jpg
│   │   └── head
│   └── bob,charlie/
│       └── ...
└── Teams/
    └── engineering/
        └── general/
            └── ...
```

## Development

```sh
# Run tests
go test ./... -count=1

# Run tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out

# Install pre-commit hooks
pre-commit install
pre-commit install --hook-type commit-msg
```

## License

MIT License — Copyright (c) 2026 Mark Ferrell. See [LICENSE](LICENSE).

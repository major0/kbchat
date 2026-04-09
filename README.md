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
| `view` | Display messages from one or more conversations |
| `grep` | Search messages across conversations |
| `help` | Show usage information |

## Usage

### export

Export Keybase chat history to a local directory.

```sh
kbchat export [options] [destdir] [filters...]
```

When `destdir` is omitted, uses `store_path` from the config file.

On each run, export fetches new messages incrementally and backfills any gaps in existing history. Messages that the API cannot return (deleted, ephemeral) are recorded as placeholders to avoid re-requesting them.

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
| `-l`, `--verbose` | Long format: type, count, size, created, modified, name |
| `--format=<fmt>` | Named format (`single-column`, `columns`, `long`) or custom format string |

Custom format tokens: `%t` (type), `%n` (name), `%c` (count), `%C` (created), `%M` (modified), `%h` (head ID), `%{field}` (named field), `%%` (literal %).

Examples:

```sh
# List all conversations
kbchat list

# List with long format (includes disk size)
kbchat list -l

# Filter by pattern
kbchat list 'Chats/*bob*' 'Teams/engineering/*'
```

### view

Display messages from one or more conversations.

```sh
kbchat view [options] <filter> [<filter> ...]
```

When multiple conversations match, output uses `==> path <==` headers and `--` separators between conversation blocks (like `head`/`tail`). A single conversation is displayed without headers.

Options:

| Flag | Description | Default |
|------|-------------|---------|
| `--count=<n>` | Number of messages per conversation (`0` for all) | 20 |
| `--date=<YYYY-MM-DD>` | Show messages from a specific day | — |
| `--after=<timestamp>` | Show messages after timestamp | — |
| `--before=<timestamp>` | Show messages before timestamp | — |
| `--verbose` | Include message IDs and metadata | off |

Timestamps accept any format supported by [dateparse](https://github.com/major0/dateparse): RFC 3339, date-only, Unix epoch, relative expressions (`3 days ago`, `last monday`), and named references (`yesterday`, `tomorrow`).

Examples:

```sh
# View last 20 messages from a DM
kbchat view Chat/alice

# View multiple conversations
kbchat view 'Chat/*bob*' Chat/carol

# View all messages from a team channel
kbchat view --count 0 Team/engineering/general

# View messages from a specific day
kbchat view --date 2025-01-15 Chat/bob

# View messages in a time range
kbchat view --after '3 days ago' --before yesterday Chat/alice
```

### grep

Search messages across conversations.

```sh
kbchat grep [options] [filters...] <pattern>
```

The default pattern mode is glob (`*` matches any characters, `?` matches one). Use `-E` for Go regexp (unanchored substring match). Searches text, edit, and headline messages; other types appear only as context lines.

Output uses `==> path <==` headers per conversation and `--` separators between conversation blocks. Non-contiguous match windows within a conversation are separated by a blank line.

Options:

| Flag | Description |
|------|-------------|
| `-E`, `--regexp` | Interpret pattern as Go regexp |
| `-i` | Case-insensitive matching |
| `-A <n>` | Show n messages after each match |
| `-B <n>` | Show n messages before each match |
| `-C <n>` | Show n messages before and after |
| `--after=<timestamp>` | Search messages after timestamp |
| `--before=<timestamp>` | Search messages before timestamp |
| `--count=<n>` | Limit total results |
| `--verbose` | Include message IDs and metadata |

Examples:

```sh
# Glob search across all conversations
kbchat grep '*deploy*'

# Regex search in a specific team
kbchat grep -E 'error|fail' Team/engineering

# Search with context
kbchat grep -C 3 'outage' Team/ops

# Case-insensitive search within a time range
kbchat grep -i --after '3 days ago' --before yesterday 'release'
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
make test

# Run tests with coverage
make coverage
make coverage-func

# Validate coverage meets 70% floor
make coverage-validate

# Run linter
make lint

# Full static analysis
make static-check

# Install pre-commit hooks
pre-commit install
pre-commit install --hook-type commit-msg
```

## License

MIT License — Copyright (c) 2026 Mark Ferrell. See [LICENSE](LICENSE).

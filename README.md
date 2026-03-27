# keybase-export

A Go CLI tool that exports Keybase chat history to a local directory tree.

Replaces the TypeScript/Deno [keybase-export](https://github.com/eilvelia/keybase-export) with a native Go implementation that uses the Keybase chat API directly via `keybase chat api`.

## Features

- Exports DMs, group chats, and team channels
- Per-message directory structure (`messages/<id>/message.json`) for O(1) lookups
- Content-addressable attachment storage (`<sha256>.<ext>`)
- Incremental backups via message-chain walking
- Concurrent downloads via goroutine worker pool
- Preserves full message history (no edit/delete collapsing)

## Requirements

- Go 1.23+
- [Keybase](https://keybase.io) CLI installed and authenticated

## Install

```sh
go install github.com/major0/keybase-export@latest
```

## Usage

```
keybase-export [options] <destdir> [filters...]
```

### Arguments

- `<destdir>` — destination directory for exported data
- `[filters...]` — optional conversation filters (`Chat/<participants>` or `Team/<team_name>`)

### Options

| Flag | Description | Default |
|------|-------------|---------|
| `-P`, `--parallel=<n>` | Number of concurrent workers | 4 |
| `--verbose` | Enable detailed logging | off |
| `--skip-attachments` | Skip downloading attachments | off |
| `--help` | Show usage | |

### Examples

Export everything:
```sh
keybase-export ~/keybase-backup
```

Export specific conversations:
```sh
keybase-export ~/keybase-backup Chat/alice Team/engineering
```

Export without attachments, 8 workers:
```sh
keybase-export -P 8 --skip-attachments ~/keybase-backup
```

## Output Structure

```
<destdir>/
├── Chats/
│   ├── alice/                    # DM with alice
│   │   ├── messages/
│   │   │   ├── 1/
│   │   │   │   └── message.json
│   │   │   └── 2/
│   │   │       ├── message.json
│   │   │       └── attachments.json
│   │   ├── attachments/
│   │   │   └── <sha256>.jpg
│   │   └── head
│   └── bob,charlie/              # Group chat
│       └── ...
└── Teams/
    └── engineering/
        └── general/
            └── ...
```

## Development

Run tests:
```sh
go test ./... -count=1
```

Run tests with coverage:
```sh
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

Pre-commit hooks are configured. Install them with:
```sh
pre-commit install
pre-commit install --hook-type commit-msg
```

## License

MIT License — Copyright (c) 2026 Mark Ferrell. See [LICENSE](LICENSE).

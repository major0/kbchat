# kbchat export

Export Keybase chat history to a local directory.

```sh
kbchat export [options] [destdir] [conversation...]
```

When `destdir` is omitted, uses `store_path` from the config file. When configured, `kbchat export` with no arguments exports everything.

On each run, export fetches new messages incrementally and backfills any gaps in existing history. Messages that the API cannot return (deleted, ephemeral) are recorded as placeholders to avoid re-requesting them.

## Options

| Flag | Description | Default |
|------|-------------|---------|
| `-P`, `--parallel=<n>` | Concurrent workers | 4 |
| `--verbose` | Detailed logging | off |
| `--skip-attachments` | Skip attachment downloads | off |
| `--continuous` | Run in a loop | off |
| `--interval=<duration>` | Sleep between cycles (with `--continuous`) | 5m |
| `--log-file=<path>` | Redirect logs to file (append mode, SIGHUP reopens) | — |

## Examples

```sh
# Export everything (uses store_path from config)
kbchat export

# Export to a specific directory
kbchat export ~/keybase-backup

# Export specific conversations
kbchat export Chat/alice Team/engineering

# Export without attachments, 8 workers
kbchat export -P 8 --skip-attachments

# Continuous export every 10 minutes
kbchat export --continuous --interval=10m

# Continuous export with log file (supports logrotate via SIGHUP)
kbchat export --continuous --log-file=/var/log/kbchat.log
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

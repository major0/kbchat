# kbchat view

Display messages from one or more conversations in IRC-log format.

```sh
kbchat view [options] <conversation> [<conversation> ...]
```

When multiple conversations match, output uses `==> path <==` headers and `--` separators between conversation blocks (like `head`/`tail`). A single conversation is displayed without headers.

## Options

| Flag | Description | Default |
|------|-------------|---------|
| `--count=<n>` | Number of messages per conversation (`0` for all) | 20 |
| `--date=<YYYY-MM-DD>` | Show messages from a specific day | — |
| `--after=<timestamp>` | Show messages after timestamp | — |
| `--before=<timestamp>` | Show messages before timestamp | — |
| `--verbose` | Include message IDs and device names | off |

Timestamps accept any format supported by [dateparse](https://github.com/major0/dateparse): RFC 3339, date-only, Unix epoch, relative expressions (`3 days ago`, `last monday`), and named references (`yesterday`, `tomorrow`).

## Examples

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

# Verbose output (message IDs and device names)
kbchat view --verbose Chat/alice
```

## Output Format

Text messages:
```
[2025-01-15 10:30:00.00 UTC] <alice> hello world
```

Non-text messages:
```
[2025-01-15 10:31:00.00 UTC] * bob edit: corrected typo
[2025-01-15 10:32:00.00 UTC] * alice reaction: 👍
[2025-01-15 10:33:00.00 UTC] * bob attachment: screenshot.png
```

Verbose mode adds message ID prefix and device name suffix:
```
[id=42] [2025-01-15 10:30:00.00 UTC] <alice> hello world (phone)
```

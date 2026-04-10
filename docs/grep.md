# kbchat grep

Search messages across conversations.

```sh
kbchat grep [options] [conversation...] <pattern>
```

The pattern is always a Go regular expression. Simple strings like `deploy` work as substring matches. Searches text, edit, and headline messages; other types appear only as context lines.

Output uses `==> path <==` headers per conversation and `--` separators between conversation blocks. Non-contiguous match windows within a conversation are separated by a blank line.

## Options

| Flag | Description |
|------|-------------|
| `-i` | Case-insensitive matching |
| `-A <n>` | Show n messages after each match |
| `-B <n>` | Show n messages before each match |
| `-C <n>` | Show n messages before and after each match |
| `--count=<n>` | Limit total results |
| `--after=<timestamp>` | Search messages after timestamp |
| `--before=<timestamp>` | Search messages before timestamp |
| `--verbose` | Include message IDs and device names |

## Examples

```sh
# Search across all conversations
kbchat grep 'deploy'

# Regex alternation in a specific team
kbchat grep Team/engineering 'error|fail'

# Case-insensitive search
kbchat grep -i 'release'

# Search with context (3 messages before and after)
kbchat grep -C 3 Team/ops 'outage'

# Search within a time range
kbchat grep --after '3 days ago' --before yesterday 'release'

# Limit to first 10 results
kbchat grep --count 10 'TODO'

# Search multiple conversations
kbchat grep 'Chat/*alice*' 'Team/engineering/*' 'deploy'
```

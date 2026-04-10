# kbchat list

List conversations in the local export store.

```sh
kbchat list [options] [conversation...]
```

Alias: `kbchat ls`

## Options

| Flag | Description |
|------|-------------|
| `-1` | One conversation per line (default when stdout is not a terminal) |
| `-C` | Column format (default when stdout is a terminal) |
| `-l`, `--verbose` | Long format: type, count, size, created, modified, name |
| `--format=<fmt>` | Named format (`single-column`, `columns`, `long`) or custom format string |

Custom format tokens: `%t` (type), `%n` (name), `%c` (count), `%C` (created), `%M` (modified), `%h` (head ID), `%{field}` (named field), `%%` (literal %).

## Examples

```sh
# List all conversations
kbchat list

# Long format (includes disk size)
kbchat list -l

# List specific conversations
kbchat list 'Chats/*bob*' 'Teams/engineering/*'

# One per line
kbchat list -1

# Custom format
kbchat list --format='%t %c %n'
```

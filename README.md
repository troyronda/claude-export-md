# claude-export-md

Convert a claude.ai data export into one Markdown file per conversation. Experimental: the API will change.

It reads the export folder (the zips, or the folders they unpack to), streams `conversations.json` one conversation at a time, and writes `<created date>-<slug>.md` files with YAML front matter. Re-running it is safe: it updates only files it wrote and that nobody has edited since.

## Install

```sh
go install github.com/troyronda/claude-export-md/cmd/claude-export-md@latest
```

## Use

Request your data from claude.ai (Settings → Privacy → Export data), download every file the manifest lists into one folder, then:

```sh
claude-export-md --dry-run <export-folder> <output-folder>   # counts only
claude-export-md <export-folder> <output-folder>
```

The export folder can hold the zips (`conversations-000.zip`, `conversations-001.zip`, …) or what they unpack to (`conversations.json`, `projects/`, `light_metadata-000/`). It is only read, never changed.

Flags:

- `--since YYYY-MM-DD`: only conversations updated on or after this date.
- `--project <name or uuid>`: only conversations in this project, when the export links conversations to projects.
- `--human <name>`, `--assistant <name>`: speaker headings. The default human heading is the first name in the export's account details.
- `--template <file>`: your own [text/template](https://pkg.go.dev/text/template) instead of the built-in format; `--print-template` shows the built-in one.
- `--allow-tracked`: see below.
- `-v`: list the ids of skipped conversations.

## What it writes

One file per conversation, `<created date>-<slug>.md` (with an id suffix if two collide), holding front matter and one `### <speaker>` heading per turn:

```markdown
---
title: 'Planning a garden shed'
kind: conversation
source: 'claude.ai'
via: claude-export-md
id: '11111111-aaaa-4bbb-8ccc-000000000001'
url: 'https://claude.ai/chat/11111111-aaaa-4bbb-8ccc-000000000001'
added: 2026-10-28
created: 2026-03-02
messages: 4
---

### Robin

How big should a shed be?

### Claude

It depends on what you store.
```

- Only the current branch of an edited conversation is written: the path to its most recent message.
- Files Claude created (`create_file`) and attachment text appear in fenced blocks. Thinking, tool calls and tool results are left out. Uploaded files whose content the export doesn't include are named.
- `updated` appears once the importer has rewritten a file because the conversation grew.

## Re-running

The importer records each file it writes, with a SHA-256 hash, in `.claude-export-md.json` in the output folder. On a later run it:

- leaves a file alone if the conversation hasn't changed;
- rewrites it if the conversation grew and the file is exactly as the importer left it;
- skips it, and counts it, if the file was edited, moved or deleted;
- never overwrites a file it didn't write.

## Git

Conversations are private. The importer refuses to write into a folder inside a git repository unless git ignores that folder and tracks nothing in it. `--allow-tracked` overrides this.

## Output

It prints counts only, never titles or text.

Built with the help of AI coding tools. Maintained by Troy Ronda.

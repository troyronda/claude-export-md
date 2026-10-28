# claude-export-md

Convert a claude.ai data export into one Markdown file per conversation. Experimental: the API will change.

It reads the export folder (the zips, or the folders they unpack to), streams `conversations.json` one conversation at a time, and writes `<created date>-<slug>.md` files with YAML front matter. Re-running it is safe: it updates only files it wrote and that nobody has edited since.

Built with the help of AI coding tools. Maintained by Troy Ronda.

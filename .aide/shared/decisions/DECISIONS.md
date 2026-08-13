# Team Decisions

This folder contains team architectural decisions, one markdown file per topic.
Each file has YAML frontmatter with structured fields and a markdown body.

## With aide

Decisions import automatically at session start when `AIDE_SHARE_AUTO_IMPORT=1` is
set in `.claude/settings.json`. Manually:

    aide share import --decisions

Decisions are append-only per topic: committing a different decision for an existing
topic supersedes the old one, and `aide decision history <topic>` shows the thread.

## Without aide

Each `.md` file is a self-contained decision record. Point your AI assistant at this
folder as context — the frontmatter answers *what* was decided, the body answers *why*.

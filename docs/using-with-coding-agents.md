# Using Postbrain with Coding Agents

This guide covers practical usage with coding agents like Codex and Claude Code.

## Recommended workflow per task

1. choose or confirm the working scope
2. run recall/context before starting work
3. perform the task
4. store key outcomes with remember
5. promote durable outcomes into knowledge artifacts

## Hook-based automation

If your agent supports hooks, wire Postbrain into:

- post-tool events (`snapshot`)
- session-end events (`summarize-session`)

This keeps memory up to date without manual overhead.

## Skill installation

Install both agent skill files when a repository is shared by multiple tools:

```bash
postbrain-cli install-codex-skill --target /path/to/project
postbrain-cli install-claude-skill --target /path/to/project
```

## Skill sync from registry

When using published team skills:

```bash
postbrain-cli skill sync --scope project:your-org/your-repo --agent claude-code
```

## Scope strategy

Use stable conventions:

- project scope for repo-specific memory
- team/company scopes for reusable standards

Avoid broad scopes for automation tasks that only need narrow access.

## Quality tips

- write concise `summary` and detailed `content`
- tag key entities (files, services, technologies, decisions)
- keep transient notes as working memory
- publish durable decisions as knowledge

For end-to-end usage patterns, see [Common Usage Workflows](./common-workflows.md).

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

### Codex hooks (non-Windows)

Codex hooks are experimental and currently disabled on Windows. On macOS/Linux, enable hooks in Codex config and add
Postbrain hook commands in `~/.codex/hooks.json` or `<repo>/.codex/hooks.json`.

Example:

```toml
# ~/.codex/config.toml
[features]
codex_hooks = true
```

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "postbrain-cli snapshot --scope project:$POSTBRAIN_SCOPE"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "postbrain-cli summarize-session --scope project:$POSTBRAIN_SCOPE"
          }
        ]
      }
    ]
  }
}
```

### Codex plugins

Codex now supports plugins in app and CLI (`/plugins`). Treat plugin usage similarly to Claude command workflows:

- install plugins for reusable workflows/integrations
- install project-local Postbrain skills for repo-specific conventions
- keep scope and token restrictions the same across both agent surfaces

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

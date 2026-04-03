# FAQ

## Do I need Postbrain if my agent already has memory?

If you need persistence across sessions, users, or agent tools, yes. Local memory is usually not enough for durable team
workflows.

## Should I store everything as memory?

No. Use memory for iteration and transient context. Promote important long-lived outcomes into knowledge artifacts.

## What scope should I use?

Use the narrowest scope that still serves the collaboration need. Start with project scope, then share upward only when
necessary.

## Can I use Postbrain with multiple agent tools?

Yes. A common pattern is to install both Codex and Claude skill files in the same repository and point both to the same
Postbrain scope.

## Is `--target` required for skill installers?

No. If omitted, current directory (`.`) is used as project root.

## Does `server.token` exist in the current config schema?

No. Current runtime config uses `server.addr`, `server.tls_cert`, and `server.tls_key` for server settings.
Authentication uses issued bearer tokens on clients (`POSTBRAIN_TOKEN`).

## Where should I start reading deeper technical docs?

- [Architecture Overview](./architecture-overview.md)
- [Configuration Reference](./configuration.md)
- [`designs/DESIGN.md`](../designs/DESIGN.md)

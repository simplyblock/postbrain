# Common Usage Workflows

This page shows practical, repeatable ways teams use Postbrain in day-to-day work.

## Workflow 1: Daily coding session memory loop

Best for: individual developers and pair-agent workflows.

1. Start session in project scope.
2. Run recall/context for the current task.
3. Implement changes.
4. Write memories for key decisions, tradeoffs, and commands.
5. End session with summarize-session.

Example commands:

```bash
postbrain-cli snapshot --scope project:acme/api
postbrain-cli summarize-session --scope project:acme/api
```

Outcome:

- next session starts faster
- less repeated debugging and rediscovery

## Workflow 2: Turn decisions into durable knowledge

Best for: architecture and API decisions that must remain stable.

1. Capture decisions as memories during implementation.
2. At checkpoint/release, publish the durable decision as a knowledge artifact.
3. Endorse/share the artifact so teams can find it quickly.

Typical pattern:

- memory while work is in progress
- knowledge artifact when decision is final enough to reuse

Outcome:

- better consistency across contributors
- fewer contradictory changes over time

## Workflow 3: Team-wide standards and runbooks

Best for: engineering standards, onboarding, and incident runbooks.

1. Store shared standards in a team/company scope.
2. Keep project-specific adaptations in project scope.
3. Recall from project scope to inherit relevant higher-level guidance.

Outcome:

- local flexibility with shared baseline standards
- easier onboarding for new team members and agents

## Workflow 4: Incident response and postmortem memory

Best for: production incidents and high-pressure troubleshooting.

1. During incident, write short high-signal memories frequently.
2. Tag entities (service names, files, components, incident ID).
3. After incident, publish a postmortem knowledge artifact.
4. Reuse artifact in future alerts and runbooks.

Outcome:

- faster repeat incident handling
- better post-incident learning retention

## Workflow 5: Multi-agent repository usage

Best for: repositories used by both Codex and Claude Code.

1. Install both skills in the same project.
2. Use one shared project scope for task memory.
3. Sync skills from registry as needed.

Example setup:

```bash
postbrain-cli install-codex-skill --target /path/to/project --url https://postbrain.example.com
postbrain-cli install-claude-skill --target /path/to/project --url https://postbrain.example.com
postbrain-cli skill sync --scope project:acme/api --agent claude-code
```

Outcome:

- consistent context across agent tools
- less tool-specific fragmentation

## Workflow 6: ChatGPT assistant with backend memory

Best for: internal product assistants or support copilots.

1. Chat app sends user query to your backend.
2. Backend queries Postbrain in the correct scope.
3. Backend sends curated context into ChatGPT prompt/tool call.
4. Backend stores useful outcomes back to Postbrain.

Outcome:

- persistent memory without exposing raw tokens client-side
- controlled scope boundaries per workspace/customer

## Workflow 7: New project bootstrap

Best for: greenfield projects.

1. Deploy Postbrain and create token.
2. Set `POSTBRAIN_URL` and `POSTBRAIN_TOKEN`.
3. Install project skill(s).
4. Define project scope naming convention early.
5. Start using recall-before-task and remember-after-task discipline.

Outcome:

- structured memory from day one
- cleaner long-term project knowledge base

## Recommended default policy

If you need one simple default policy:

- recall before every non-trivial task
- remember after every meaningful change
- publish durable outcomes at natural checkpoints
- keep scopes narrow by default

# Using Postbrain with ChatGPT

You can use Postbrain with ChatGPT workflows in two common ways.

## Option 1: direct tool integration (MCP-capable environments)

If your ChatGPT environment supports tool connectors/MCP, connect Postbrain directly and use the same memory flow:

- start session
- recall context
- execute task
- remember outcomes
- close session

## Option 2: backend proxy integration

If direct tool integration is not available, route requests through your backend:

- ChatGPT calls your backend
- your backend calls Postbrain APIs with scoped credentials
- your backend returns context/results to ChatGPT

This is common for internal assistants embedded in products.

## Best practices

- use narrow scopes by workspace/project
- avoid exposing raw tokens in client-side code
- write concise summaries for faster recall quality
- promote durable guidance into knowledge artifacts

## When Postbrain adds the most value

- long-running projects with many sessions
- shared team assistants
- high need for stable decisions and institutional memory

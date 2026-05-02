# MCP Ratchet – Simple Overview

## What is MCP Ratchet?

MCP Ratchet is a small, smart helper that runs inside an MCP server. Its job is to make sure the AI, or large language model (LLM), follows the exact steps you want every time before it is allowed to use certain tools.

It acts like a strict but helpful gatekeeper that enforces good habits.

## Why it is needed

LLMs are very smart, but they can be forgetful or take shortcuts. For example:

- You may want the AI to always check existing tags before creating a new cell.
- Without help, the AI may forget, invent new tags, or use irrelevant ones.
- This can create messy data that is hard to search or organize later.
- Simply telling the AI to remember to check tags first in the prompt usually does not work reliably.

MCP Ratchet solves this by making the rule impossible to break.

## What it does

It creates a simple “you must do A before you can do B” system:

1. The AI wants to use a tool such as `Create Cell`.
2. Before it is allowed to do that, it must first run a required earlier tool, such as `List Tags`.
3. When the AI successfully runs `List Tags`, the server gives it a special one-time code called a ratchet token.
4. The AI must include that exact code when it tries to call `Create Cell`.
5. After `Create Cell` runs successfully, the old code expires and a new one is issued for next time.
6. If the AI tries to create a cell without the correct code, the server politely refuses and tells it exactly what to do first.

## What it enforces

MCP Ratchet helps enforce:

- Correct order of operations.
- Required preparation steps, such as checking data, listing options, or validating inputs.
- Consistent behavior across all sessions.
- Better data quality, including proper tagging and complete context.

It is not mainly about stopping dangerous actions. It is about making sure the AI follows a preferred workflow.

## Example

Without MCP Ratchet, an AI might create cells with random or irrelevant tags, which leads to messy data.

With MCP Ratchet:

1. The AI must call `List Tags` first.
2. It gets the current tags and a secret code.
3. It can then call `Create Cell`, but only if it provides the code.

The result is that every cell is more likely to be tagged properly using real existing tags.

## Benefits

- The AI learns the correct pattern very quickly.
- Results stay cleaner and more organized.
- There is no need to write long, repetitive instructions in every prompt.
- It works reliably even when the model is creative or distracted.

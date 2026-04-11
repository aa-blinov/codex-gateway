# codex-gateway

OpenAI-compatible HTTP wrapper around [`codex exec`](https://github.com/openai/codex).

## Why

`codex exec` is a capable agentic CLI — it can reason, use tools, and produce structured output. But it has no HTTP API, which means it can't be used with the broader ecosystem of OpenAI-compatible clients: Cursor, Open WebUI, LangChain, LlamaIndex, or anything that speaks `/v1/chat/completions`.

`codex-gateway` bridges that gap. It exposes a standard OpenAI-compatible API and translates each request into a `codex exec` subprocess call, so any client that works with OpenAI works with Codex too — no code changes needed.

## Requirements

- [`codex`](https://github.com/openai/codex) installed and on `PATH` (`npm install -g @openai/codex`)
- A configured `~/.codex/config.toml`

## Installation

Download the binary for your platform from [Releases](https://github.com/aa-blinov/codex-gateway/releases/latest):

| Platform | File |
|----------|------|
| Windows x64 | `codex2api-windows-amd64.exe` |
| Windows ARM64 | `codex2api-windows-arm64.exe` |
| Linux x64 | `codex2api-linux-amd64` |
| Linux ARM64 | `codex2api-linux-arm64` |
| macOS x64 | `codex2api-darwin-amd64` |
| macOS Apple Silicon | `codex2api-darwin-arm64` |

Or build from source (requires Go 1.22+):

```bash
go build -o codex2api .
```

## Usage

```bash
./codex2api --port 8000 --cwd /path/to/your/project
```

Point any OpenAI-compatible client at `http://127.0.0.1:8000` with any API key (not validated).

### Options

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--host` | `CODEX_CHAT_WRAPPER_HOST` | `127.0.0.1` | Listen address |
| `--port` | `CODEX_CHAT_WRAPPER_PORT` | `8000` | Listen port |
| `--cwd` | `CODEX_CHAT_WRAPPER_CWD` | current dir | Working directory for `codex exec` |
| `--codex-config` | `CODEX_CHAT_WRAPPER_CODEX_CONFIG` | `~/.codex/config.toml` | Path to Codex config |
| `--max-concurrency` | `CODEX_CHAT_WRAPPER_MAX_CONCURRENCY` | `100` | Max parallel `codex exec` processes |
| `--timeout-seconds` | `CODEX_CHAT_WRAPPER_TIMEOUT_SECONDS` | `600` | Per-request timeout |

## API

### `GET /v1/models`

Returns models discovered from `~/.codex/config.toml` (top-level `model` and per-profile `model` values). Override with `CODEX_CHAT_WRAPPER_MODELS=model-a,model-b`.

### `POST /v1/chat/completions`

Standard OpenAI chat completions. Supports `model@preset` syntax to select a Codex profile:

```bash
curl http://127.0.0.1:8000/v1/chat/completions \
  -H 'content-type: application/json' \
  -d '{
    "model": "gpt-5.4",
    "reasoning_effort": "low",
    "messages": [{"role": "user", "content": "Summarize this repo"}]
  }'
```

### `POST /v1/responses`

Stateless subset of the OpenAI Responses API:

```bash
curl http://127.0.0.1:8000/v1/responses \
  -H 'content-type: application/json' \
  -d '{
    "model": "gpt-5.4",
    "instructions": "Reply briefly.",
    "input": "What is 2+2?"
  }'
```

### `GET /healthz`

Health check — returns `{"status":"ok"}`.

## Model & preset syntax

`model` can be either a plain model name or `model@preset`:

```
gpt-5.4              → codex exec -m gpt-5.4
gpt-5.4@fast         → codex exec -m gpt-5.4 -p fast
```

Profiles are defined in `~/.codex/config.toml` and can preset reasoning effort, sandbox policy, MCP setup, and other Codex options.

## Structured output

Pass `response_format.type = "json_schema"` (chat completions) or `text.format.type = "json_schema"` (responses) to enforce a JSON Schema on the output. The schema is written to a temp file and forwarded to `codex exec --output-schema`.

## Not supported

- Streaming (`stream: true`)
- Tools / function calling
- Sampling parameters (`temperature`, `top_p`, etc.)
- Multi-turn server-side state (`previous_response_id`)
- `n > 1`

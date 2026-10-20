# codex-gateway

An OpenAI-compatible HTTP API in front of `codex exec`, the Codex CLI. Point
any client that speaks `/v1/chat/completions` at it, and every request
becomes one `codex exec` subprocess that runs with your Codex login, your
profiles and your configuration. The answer comes back in the shape the
client expects.

[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Release](https://img.shields.io/github/v/release/aa-blinov/codex-gateway.svg)](https://github.com/aa-blinov/codex-gateway/releases/latest)

---

## Quick start

```bash
npm install -g @openai/codex    # the CLI the gateway drives
go build -o codex2api .          # or take a release binary
./codex2api --port 8000 --cwd /path/to/your/project
```

`codex` has to be on `PATH`, and Codex has to be configured: the gateway
reads the list of models out of `~/.codex/config.toml` and hands the rest of
the work to the CLI.

Ask it something:

```bash
curl http://127.0.0.1:8000/v1/chat/completions \
  -H 'content-type: application/json' \
  -d '{"model":"gpt-5.4","messages":[{"role":"user","content":"Summarize this repo"}]}'
```

Point the client at `http://127.0.0.1:8000` with any API key. The key is not
checked, and the server has no authentication of its own, which is why the
default address is `127.0.0.1`.

### Release binaries

| Platform | File | Size in v0.1.0 |
| --- | --- | --- |
| macOS, Apple Silicon | `codex2api-darwin-arm64` | 6,121,250 B |
| macOS, Intel | `codex2api-darwin-amd64` | 6,606,656 B |
| Linux x64 | `codex2api-linux-amd64` | 6,508,706 B |
| Linux ARM64 | `codex2api-linux-arm64` | 6,098,976 B |
| Windows x64 | `codex2api-windows-amd64.exe` | 6,705,664 B |
| Windows ARM64 | `codex2api-windows-arm64.exe` | 6,078,976 B |

They come from the [v0.1.0 release](https://github.com/aa-blinov/codex-gateway/releases/tag/v0.1.0),
published on 11 April 2026.

## What it does

- **Chat completions.** `POST /v1/chat/completions` takes `system`,
  `developer`, `user` and `assistant` messages, with content as a string or
  as a list of text parts, and answers with one choice.
- **A stateless slice of the Responses API.** `POST /v1/responses` takes
  `instructions` and an `input` that is a string or a list of items, and
  answers with one `output_text` message.
- **A model list taken from your Codex config.** `GET /v1/models` returns
  every `model = "..."` line in `~/.codex/config.toml`, the ones under a
  profile included.
- **A Codex profile per request.** A model name of `gpt-5.4@fast` becomes
  `codex exec -m gpt-5.4 -p fast`, so the profile decides the sandbox policy
  and the rest of the Codex options.
- **A reasoning effort of low, medium or high.** `reasoning_effort`, or
  `reasoning.effort`, becomes the Codex override
  `model_reasoning_effort="high"`.
- **JSON Schema output.** `response_format.type = "json_schema"`, or
  `text.format.type` on the Responses API, is written to a temporary file and
  passed to `codex exec --output-schema`.
- **A health check.** `GET /healthz` answers `{"status":"ok"}`.
- **A refusal for everything else.** Streaming, `tools`, `tool_choice`, the
  sampling parameters, `n` above 1 and `previous_response_id` come back as a
  400 that names the fields it turned down.

## How it works

```
request -> validate -> build the prompt -> codex exec -> read the -o file -> JSON
```

The prompt is assembled by `buildPrompt`. The system and developer messages
go into a `<SYSTEM_MESSAGES>` block, everything else into `<CONVERSATION>`,
and a three-line preamble goes on top: one assistant reply, no role labels,
no wrapper commentary. The whole prompt goes to the subprocess on standard
input.

The answer arrives in a file rather than on standard output. The gateway
passes `-o <path>`, `codex exec` writes its reply there, and the gateway
reads the file and puts the contents in the response. The text goes out as it
is, with nothing trimmed and nothing added.

The model list is not a TOML parse. One regular expression runs over the
config file and keeps every `model = "..."` line it finds, in the order it
finds them, with duplicates dropped. The names in `CODEX_CHAT_WRAPPER_MODELS`
are placed in front of them. The file is read on every call to
`/v1/models`, so a change to the config shows up without a restart.

Concurrency is a counting semaphore of `--max-concurrency` slots. A request
takes a slot, runs one `codex exec` and gives the slot back. The timeout
context is created after the slot is granted, so the wait in the queue is
outside the request's own 600 seconds, and it descends from the HTTP request,
so a client that closes the connection stops the subprocess.

Each subprocess is started with `--ephemeral`, so the CLI keeps no session of
it, with `--skip-git-repo-check`, so the gateway can work in a directory that
is not a git repository, and with `--color never`, so the answer is plain
text.

## Configuration

| Flag | Environment variable | Default | What it does |
| --- | --- | --- | --- |
| `--host` | `CODEX_CHAT_WRAPPER_HOST` | `127.0.0.1` | The address the server listens on. |
| `--port` | `CODEX_CHAT_WRAPPER_PORT` | `8000` | The port the server listens on. |
| `--cwd` | `CODEX_CHAT_WRAPPER_CWD` | the current directory | The directory `codex exec` runs in. |
| `--codex-config` | `CODEX_CHAT_WRAPPER_CODEX_CONFIG` | `~/.codex/config.toml` | The file the model list is read from. |
| `--max-concurrency` | `CODEX_CHAT_WRAPPER_MAX_CONCURRENCY` | `100` | How many `codex exec` processes may run at once. |
| `--timeout-seconds` | `CODEX_CHAT_WRAPPER_TIMEOUT_SECONDS` | `600` | The timeout of one request, in seconds. |

`CODEX_CHAT_WRAPPER_MODELS` has no flag. It holds a comma-separated list of
model names, and those names are listed before the ones from the config file.
A `~` at the start of the config path is expanded to the home directory.

Both `--max-concurrency` and `--timeout-seconds` have to be at least 1, and
anything smaller stops the process at startup.

## Development

```bash
go build ./...       # compile
go test ./...        # the tests
gofmt -l .           # the files that are not formatted
go vet ./...         # vet
make build           # dist/codex2api, for this machine
make build-all       # the six release binaries, in dist/
make clean           # remove dist/
```

The module has no dependencies, so `go build` and `go test` download nothing
and need no network. `go test ./...` runs `main_test.go`, which covers the
parsing and the prompt building; it never starts a `codex exec`, so it passes
on a machine with no Codex installed.

| Path | What it holds |
| --- | --- |
| `main.go` | The whole server: flags, routes, validation, the prompt, the subprocess, the JSON responses. |
| `main_test.go` | The tests for the functions that take a request apart. |
| `Makefile` | `build`, `build-all` and `clean`. |
| `go.mod` | Module `codex2api`, Go 1.22, no dependencies. |

The Go module and the binary are called `codex2api`; the repository is
`codex-gateway`. The release assets and the `owned_by` field in
`/v1/models` carry the first name, so a rename has to reach all of them.

## License

MIT, © Alexander Blinov. See [LICENSE](LICENSE).

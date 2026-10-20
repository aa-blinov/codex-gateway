# Architecture

How `codex-gateway` is put together.

The gateway is one Go program with one source file. It listens on four
routes, turns each request into a prompt, runs one `codex exec` subprocess
and answers with JSON. There is no database, no session and no state beyond
the flags it was started with.

## The shape

`main.go` holds the server in 704 lines, and `main_test.go` holds 289 lines
of table-driven tests in the same package. The table that follows is by
responsibility rather than by file, because there is only one file.

| Part | What it is responsible for |
| --- | --- |
| The server state, `srv` | The working directory, the path of the config file, the timeout and the semaphore that counts the running processes. |
| The request error, `reqErr` | The status code and the code of a failure, with the three constructors: bad request, server error and timeout. |
| The JSON helpers | The response header, the encoder and the error object. |
| The validators | The fields the gateway refuses, and the message that names them. |
| The message builder | `buildPrompt`, `renderMessage` and `normalizeContent`: the prompt the CLI receives. |
| The schema extractor | The JSON Schema, wherever the client put it: `response_format` or `text.format`. |
| The input builder | `buildResponsesMessages` and `coerceInputItem`: the input of the Responses API, turned into messages. |
| The model discovery | `loadModels`: the model names, from the environment variable and from the config file. |
| The subprocess | `runCodex`: the `codex exec` call, the temporary directory, the output file. |
| The handlers | `healthz`, `listModels`, `chatCompletions` and `responses`: the four routes. |
| The flag block in `main()` | The six flags, their environment variables, their defaults and the two checks at startup. |
| `main_test.go` | The tests for the pure functions. Nothing in them starts a server or a subprocess. |
| `Makefile` | `build`, `build-all` and `clean`. The binaries land in `dist/`. |

## The data flow

```
HTTP request
 → decode the JSON body
 → validate, and refuse the fields the gateway cannot honour
 → parse the model and the profile, the reasoning effort, the schema
 → build the prompt
 → take a slot from the semaphore
 → start the timeout
 → codex exec: the prompt on standard input, the answer in the output file
 → read the output file
 → JSON answer
```

The two answer routes share everything from the prompt onwards. They differ
in the validation, in the way the input is read and in the shape of the
response.

## Decisions worth knowing about

### The answer comes from the output file, not from stdout

`codex exec` writes its reply to the path given by `-o`. Standard output
carries the progress log. Reading stdout by mistake hands the client the
log, so the gateway reads the file and puts its contents in the response.

### The prompt goes in on standard input

The last argument of the command is `-`, and that is what tells the CLI to
read the prompt from standard input. Without it the CLI waits for a
terminal.

### The timeout starts when the slot is granted

The send on the semaphore blocks before the context is created, so the wait
in the queue sits outside the 600 seconds of the request, and those seconds
are counted from the moment a slot is free. The context is then derived
from the request context, so a client that disconnects kills the
subprocess.

### The model list is a regular expression, not a TOML parse

`modelLineRe` matches any line that starts with `model = "..."`, at any
nesting level, and only with double quotes. A single-quoted model line is
invisible to `/v1/models`, and a `model` key under a table that is not a
profile is listed as if it were a model. The file is read on every call, so
a change to the config shows up without a restart.

### The names from the environment come first

`CODEX_CHAT_WRAPPER_MODELS` is split on commas and put in front of the
names from the config file. The first occurrence of a duplicate wins, so
the variable overrides the file rather than adding to it.

### The reasoning effort travels as a config override

Low, medium and high are the only values accepted. The effort reaches the
CLI as `-c model_reasoning_effort="high"`, which is a config override, and
not as a flag of `codex exec`.

### A schema is a file with mode 0600

The JSON Schema is written to a temporary directory next to the output file
and passed as `--output-schema`. The directory is removed when the call
ends.

### The module has no dependencies

`net/http`, `encoding/json`, `os/exec` and `regexp` are enough for all of
it. `go build` and `go test` download nothing and need no network, and the
binary is a single file that copies to a machine with nothing else on it.

## Boundaries

- Everything is one package. The tests are in the same package, so they
  reach the unexported functions directly.
- The pure functions take data and return data or an error. `buildPrompt`,
  `loadModels`, the validators and the parsers touch neither the server
  state nor the process, and that is what lets the suite run on a machine
  with no `codex` installed.
- `runCodex` is the only function that starts a process. Everything above
  it can be exercised without one.
- The HTTP layer owns the JSON and nothing else. The prompt layer owns the
  text and nothing else.
- The handlers do not call each other. Each one goes from the request
  straight to the response.

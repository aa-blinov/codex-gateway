# AGENTS.md — agent context for `codex-gateway`

An OpenAI-compatible HTTP server that turns a request into one `codex exec`
subprocess and returns the answer in the OpenAI shape. The server is
`main.go`, the tests are `main_test.go`, and there is nothing else in the
source tree.

This file is what you need to change the project without reading all of it.

## Run / build / test

```bash
go build ./...       # compile
go test ./...        # the tests
gofmt -l .           # the files that are not formatted
go vet ./...         # vet
make build           # dist/codex2api, for this machine
make build-all       # the six release binaries, in dist/
make clean           # remove dist/
```

Go 1.22 is what `go.mod` asks for. The module has no dependencies, so
nothing is downloaded and the tests need no network.

`go test ./...` covers the pure functions: the model and preset parsing, the
reasoning effort, the request validation, the content normalisation, the
prompt building, the schema extraction and the model discovery. Nothing in
the suite starts an HTTP server or a `codex exec`, so the handlers and the
subprocess call are uncovered. There is no test that needs a real `codex`
binary, and a change to `runCodex` has to be checked by hand.

## Where things live

| Path | What it holds |
| --- | --- |
| `main.go` | The server, in 704 lines: flags, routes, validation, the prompt, the subprocess, the JSON responses. |
| `main_test.go` | The tests, in the same package, table-driven, 289 lines. |
| `Makefile` | `build`, `build-all` and `clean`. Writes to `dist/`, which `.gitignore` ignores. |
| `go.mod` | Module `codex2api`, `go 1.22`, no `require` block. |

## Traps

- **The answer comes from the `-o` file, not from stdout.** `codex exec`
  writes its reply to the path given by `-o`; stdout carries the progress
  log. Read stdout by mistake and the client gets the log.
- **The prompt goes in on stdin, and the last argument is `-`.** The `-` is
  what tells the CLI to read the prompt from standard input. Without it the
  CLI waits for a terminal.
- **`--ephemeral` and `--skip-git-repo-check` are always passed.** The first
  keeps the CLI from keeping a session, the second lets the gateway work
  in a directory that is not a git repository, which is what makes `--cwd`
  worth having. Removing either changes behaviour that the code does not
  show.
- **`-X main.version=$(VERSION)` in the Makefile sets nothing.** There is no
  `version` variable in `main.go`, and the linker ignores `-X` for a symbol
  that does not exist. `make build VERSION=1.2.3` produces a binary that
  reports nothing about its version.
- **`main_test.go` is not gofmt-clean.** `gofmt -l .` prints it: the fields
  of the table struct are aligned by hand and gofmt wants one space less. A
  formatting check fails on that file alone.
- **The model list is a regular expression, not a TOML parse.** `modelLineRe`
  matches any line that starts with `model = "..."`, at any nesting level,
  and only with double quotes. `model = 'gpt-5.4'` is invisible to
  `/v1/models`, and a `model` key under a table that is not a profile is
  listed as if it were a model.
- **`CODEX_CHAT_WRAPPER_MODELS` prepends, it does not replace.** The names
  from the variable come first, the names from the config file follow, and
  the first occurrence of a duplicate wins.
- **The timeout starts when the slot is granted.** The semaphore send blocks
  before the context is created, so a request waiting behind 100 running
  processes waits outside the timeout, and the 600 seconds are counted from
  the moment the slot is free.
- **The timeout context descends from the request context.**
  `context.WithTimeout(r.Context(), ...)` means a client that disconnects
  kills the subprocess. That is deliberate; do not detach it.
- **`n` is allowed only when it is 1, and it is ignored.** The response always
  carries exactly one choice.
- **The `model@preset` split is on the first `@`.** `SplitN(name, "@", 2)`
  keeps the rest of the string as the preset name, and an empty side on
  either end is a 400.
- **Only low, medium and high are accepted as a reasoning effort.** The
  effort reaches Codex as a config override,
  `-c model_reasoning_effort="high"`, and not as a flag of `codex exec`.
- **Unsupported fields are refused by name.** A field the gateway cannot
  honour has to be added to `checkUnsupported` in both validators, or the
  client waits for a behaviour that never comes.
- **The system and developer roles are moved out of the conversation.**
  `buildPrompt` puts them in `<SYSTEM_MESSAGES>`, everything else in
  `<CONVERSATION>`, and refuses a request that carries no user or assistant
  message.
- **The text of the answer is not the gateway's to change.** The contents of
  the `-o` file go into the response as they are.

## Style / safety rails

- **The module has no dependencies, and that is a rule.** The standard
  library covers it: `net/http`, `encoding/json`, `os/exec`, `regexp`. A
  dependency needs a reason written next to it.
- **The API key is not checked.** The server reads no `Authorization` header.
  Validating keys changes the contract with every client that sends "any
  key", which is what the README tells them to do.
- **The default bind address is `127.0.0.1`, and there is no authentication,
  no TLS and no CORS.** The process starts a `codex exec` with the
  permissions of the user who started it, in the directory given by
  `--cwd`. Do not change the default to `0.0.0.0`.
- **Comments explain why, never what.** The banner comments in `main.go` are
  the only decoration in the file.
- **No emoji**, in any file.
- **The CI workflow the standard brings is a Python placeholder.** The
  adoption branch's `.github/workflows/ci.yml` still runs
  `echo "Replace with the test command for this stack."`. It wants
  `go build ./...`, `go test ./...` and `gofmt -l .`, and its `push` filter
  says `main` while this repository's branch is `master`.

## Where the product truth lives

`PRODUCT.md` holds it. Read it before changing what the gateway refuses, and
write the change there when the refusal changes.

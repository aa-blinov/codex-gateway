# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

The product is an HTTP server written in Go 1.22, with nothing outside the
standard library. It runs as one process on the machine of the person who
started it, and it has no interface of its own: the whole surface is four
routes and the JSON they return.

The routes are `GET /healthz`, `GET /v1/models`, `POST /v1/chat/completions`
and `POST /v1/responses`.

## Users

The person who already runs `codex exec` from a terminal and wants the same
thing from a client that speaks only the OpenAI API: an IDE extension, a
chat interface, a notebook, a script. The situation is specific: the CLI is
capable and already signed in, and the only way into it is a subprocess,
while the client wants an endpoint it can talk to.

A second audience is not confirmed. The repository has 0 stars, 0 forks, one
open issue (the adoption of the house standard) and one release, so the only
user the evidence supports is the owner.

## Product Purpose

The gateway does one thing: it takes a request in the OpenAI shape, runs one
`codex exec` with it, and returns the answer in the OpenAI shape.

The request is not forwarded to a model API. It is turned into a prompt, the
prompt goes to the Codex CLI, and the answer the CLI produced comes back.
Codex's login, its profiles, its sandbox policy and its tools are used as
they are, because the CLI is the thing that runs.

Success for the user: a client that knows `/v1/chat/completions` gets answers
from Codex, and nothing has to change on the client side. Success for the
product: the request arrives, one subprocess runs, the answer comes back
unchanged, and the client cannot tell the difference between this and an
OpenAI endpoint, except in the fields the gateway refuses.

## Positioning

The claim a neighbouring product could not copy is the agent. `codex exec` is
a full agent: it runs shell commands, reads files and edits them by itself.
The gateway neither reimplements that nor tries to. It puts an HTTP
interface in front of a program that is already installed, already
authenticated and already configured on that machine.

The second difference is one of direction. A proxy such as LiteLLM speaks the
client-side tool loop: the model asks for a tool, the client runs it, the
result goes back. That loop cannot be reproduced here, because `codex exec`
keeps its tool calls to itself. The gateway says so in the README rather than
accepting a request it cannot honour.

## Operating Context

The gateway is started by hand and left running:

```bash
./codex2api --port 8000 --cwd /path/to/your/project
```

One process, one working directory, one Codex configuration. The directory
given by `--cwd` is the directory the agent works in, so serving a second
project means starting a second process on another port.

The client is configured with the base URL and an API key it does not have to
invent, because the key is not checked. The default bind address is
`127.0.0.1`, so the server is reachable from the machine it runs on and from
nowhere else, and it has no authentication, no TLS and no CORS.

One request holds one slot out of `--max-concurrency` (100 by default) and
runs one `codex exec` process, and is answered within `--timeout-seconds`
(600 by default). The queue in front of the semaphore is outside that
timeout.

## Capabilities and Constraints

Confirmed by the code:

- `POST /v1/chat/completions` and `POST /v1/responses`, with the system,
  developer, user and assistant roles, and content as a string or a list of
  text parts.
- `GET /v1/models`, from the `model = "..."` lines of the Codex config file,
  with the names in `CODEX_CHAT_WRAPPER_MODELS` placed in front of them.
- `model@preset` in the model name, which becomes
  `codex exec -m model -p preset`.
- A reasoning effort of low, medium or high, as `reasoning_effort` or
  `reasoning.effort`, which becomes `-c model_reasoning_effort="high"`.
- A JSON Schema in `response_format`, or in `text.format`, written to a
  temporary file with mode 0600 and passed as `--output-schema`.
- `GET /healthz`, which answers `{"status":"ok"}`.
- The answer is the file `codex exec` wrote, unchanged.
- Errors in the OpenAI shape, with `error.message`, `error.type` and
  `error.code`, and the codes `invalid_request_error`, `server_error`,
  `codex_exec_failed`, `missing_output` and `timeout`.

Deliberately not true, and worth knowing before a change:

- Streaming is refused. The answer arrives whole.
- `tools`, `tool_choice` and `parallel_tool_calls` are refused, because the
  agent's tool calls cannot be re-exposed as OpenAI tool calls.
- `temperature`, `top_p`, `max_tokens`, `max_completion_tokens`,
  `presence_penalty`, `frequency_penalty`, `logprobs` and `top_logprobs` are
  refused. The sampling parameters of the API have no equivalent here.
- `n` above 1 is refused, and `n` of 1 is accepted and ignored.
- `previous_response_id` and `conversation` are refused: there is no
  server-side state and no store to keep it in.
- The API key is not checked, and the `Authorization` header is never read.

Not answered, and for the owner to answer:

- Which clients are pointed at it. Nothing in the repository records a
  client.
- Whether the gateway should be a service with a configuration file rather
  than a process started by hand. The flags and the environment variables are
  the whole configuration today.
- Whether the answer should ever be trimmed or reformatted. It is passed on
  as it is now.

## Brand Commitments

**Name:** codex-gateway. The Go module, the binary and the release assets
are called `codex2api`, and `/v1/models` reports `owned_by: "codex2api"`.
Both names stay.

**Tone:** plain and technical. The README says what the gateway cannot do
and why, in the section that says it.

**A failure is visible.** A refused request names the fields it refused. A
failed subprocess carries the stderr of `codex exec` in the error message. A
missing output file has its own code, `missing_output`, and not a generic
failure.

## Evidence on Hand

- The README that was in the repository: the motivation, the release table,
  the flags, and the section on why tools are not supported.
- `main.go`, 704 lines: the routes, the validation, the prompt, the
  subprocess call, the response shapes.
- `main_test.go`, 289 lines: the tests for the parsing, the validation, the
  prompt and the model discovery.
- `Makefile`: `build`, `build-all`, `clean`, and the six release targets.
- The v0.1.0 release of 11 April 2026: six binaries, 6,078,976 B to
  6,705,664 B.
- The repository description: "OpenAI-compatible HTTP wrapper over codex exec".
- One open issue, "Adopt the house standard", and one pull request that
  adds the standard's files.

Absent, and not to be invented: no download counts, no usage numbers, no
testimonials, no comparison with other gateways, and no record of which
clients use it. The repository has 0 stars and 0 forks, so there is nothing
to count. The repository carried no licence file before the house standard
was adopted, and the licence it carries now is the collection's MIT.

## Product Principles

1. **The Codex agent stays whole.** The gateway translates a request and
   hands the work to `codex exec`. It does not reimplement the tools the
   agent runs, and it does not try to expose them.
2. **A capability the gateway cannot honour is refused, not ignored.** Every
   unsupported field comes back as a 400 that names it, because a client
   that is silently given something else is worse than one that is told.
3. **The answer is the answer.** The contents of the file `codex exec`
   wrote go into the response unchanged, and the gateway does not trim them
   or explain them.
4. **One process, one working directory.** The directory the agent works in
   is set when the gateway starts and does not change while it runs.
5. **The boundary is written down.** The README says what is not supported
   and why, in the same words the code refuses it.

## Accessibility & Inclusion

The product has no interface of its own, so the accessibility surface is the
client's, and the gateway cannot set a standard for it. The gateway owns the
shape of its errors, and they are sentences: the 400 names the fields it
refused, and the 500 carries the output of `codex exec` rather than a code
on its own.

The limits of the product belong in the same place:

- Text only. An `image_url` content part is refused, so a client with
  multimodal input cannot use the gateway as it stands.
- The transcript is text, and a long conversation is passed to the CLI as
  one prompt. The code sets no limit on the length of that prompt and does
  no truncation either.
- A refusal is an HTTP 400 with a JSON body. A client that shows only the
  status code shows the user nothing about what to change.

No accessibility standard has been set for the client side, because the
gateway does not own the client. Whether the gateway should answer with an
error shape a screen reader reads well, for a client that is not known, is an
open question.

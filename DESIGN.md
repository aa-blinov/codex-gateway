---
name: codex-gateway
description: The JSON on the wire. The gateway has no interface of its own, so its design is four response shapes, one prompt shape and the sentences in the errors.
colors:
  primary: none
  neutral-bg: none
typography:
  body:
    fontFamily: "plain text, no font"
    fontSize: "no size"
    fontWeight: "no weight"
    lineHeight: "no leading"
  title:
    fontFamily: "plain text, no font"
    fontSize: "no size"
    fontWeight: "uppercase"
    lineHeight: "no leading"
rounded:
  md: none
spacing:
  sm: "one space"
  md: "two newlines between the prompt blocks"
---

# Design system: codex-gateway

## Overview

**Creative North Star: "an invisible translator"**

The gateway has no interface. There is no page, no window, no image, no
stylesheet and no colour anywhere in the repository. The only thing a person
sees is the JSON that a client they already have renders.

So the design surface is the wire: the four routes, the shape of what each
one answers with, the shape of the prompt the CLI receives, and the
sentences in the errors. A client that knows the OpenAI API should not be
able to tell the gateway apart from an OpenAI endpoint, and the one place
where it does differ is the refusals, which name the fields they turned
down.

What the project rejects: no branding, no wrapper around the answer, no
trimming of what the CLI wrote, and no field that is quietly ignored.

Key characteristics:

- The answer is never dressed up.
- A refusal names the field it refused.
- Every identifier has a fixed prefix and a fixed length.
- The prompt is one text with three blocks.

## Colors

The project defines no palette, because the response is JSON. `Content-Type`
is `application/json` and nothing else is sent: no stylesheet, no class, no
theme. The client that renders the answer decides how it looks.

- **The client's palette.** The gateway sends data. Where the data appears,
  and in what colour, is the client's business.
- **No image of any kind.** Not in the responses, not in the errors.

### Named rules

**The client paints it.** Do not add a style, a wrapper or a field that only
carries presentation. A field the OpenAI shape does not have is a field a
client has to ignore.

**One sentence per refusal.** A refusal names the fields it turned down and
stops there. It does not suggest a workaround, because the gateway does not
know what the client can do.

## Typography

The prompt is the only formatted text the project writes. It is plain text
with a fixed shape, and the CLI reads it as it stands.

- **The preamble.** Three lines, one sentence each, no markup. It tells the
  model to answer with one assistant reply, without role labels, XML tags or
  wrapper commentary. With a schema in play, a fourth line says the answer
  has to match the schema.
- **The block headers.** `<SYSTEM_MESSAGES>` and `<CONVERSATION>`, in upper
  case and in angle brackets. They are the two section markers of the
  prompt.
- **The message header.** The role in upper case and a colon, and the
  content on the lines below it: `USER:`. A message that carries a name
  gets it in brackets: `USER (alice):`.

The JSON side has no typography at all. The keys are the lower snake case
names OpenAI uses, and the gateway does not rename them.

## Layout

Four routes, and each one answers with one JSON object.

| Route | What it answers with |
| --- | --- |
| `GET /healthz` | `{"status":"ok"}` |
| `GET /v1/models` | A list object with one entry per model name found. |
| `POST /v1/chat/completions` | A chat completion with exactly one choice. |
| `POST /v1/responses` | A response with one output message. |

The chat completion carries `id`, `object`, `created`, `model` and
`choices`. The choice carries `index`, `message.role`, `message.content` and
`finish_reason`, which is always `stop`. The `model` field echoes what the
client asked for, the `@profile` suffix included.

The responses answer carries `id`, `object`, `created_at`, `status`,
`error`, `incomplete_details`, `instructions`, `model` and `output`. The
output message carries `content[0].type`, which is `output_text`, and the
text of the answer in `content[0].text`.

The model list carries `object`, which is `list`, and a `data` array. Each
entry has an `id`, an `object` of `model`, a `created` of `0` and an
`owned_by` of `codex2api`.

Identifiers have a prefix and a fixed length. A chat completion is
`chatcmpl-` followed by 32 hexadecimal characters, a response is `resp_`
followed by 32, and an output message is `msg_` followed by 32. The 32
characters are 16 random bytes from the system entropy source.

## Elevation & depth

Nothing is layered. Every response is one JSON object, and the only nesting
is in the two answer routes, where it is the nesting the OpenAI shape has.
A client parses it without knowing that a gateway answered.

## Shapes

Two shapes carry the whole contract.

- **The answer.** The OpenAI shape, unchanged. The gateway adds no field of
  its own to a successful response.
- **The error.** `{"error": {"message": ..., "type": ..., "code": ...}}`.
  The `type` and the `code` carry the same value, so a client can read
  either one.

There is no rounding, no clipping and no case folding. The message of an
error is a sentence, and it is the only place where the gateway writes prose
that a person reads.

## Components

### The error object

One object, three fields. The message says what went wrong, the code says
which kind: `invalid_request_error`, `server_error`, `codex_exec_failed`,
`missing_output` or `timeout`.

### The chat completion answer

Five fields at the top, one choice inside, one message inside the choice.
The content of the message is the text that `codex exec` wrote, with
nothing trimmed away and nothing put in.

### The responses answer

Ten fields at the top, one output message inside, one content part inside
the message. The `instructions` field echoes what the client sent.

### The model entry

Four fields. The `created` field is `0` because the gateway does not track
when a model appeared.

### The prompt

Three blocks in a fixed order: the preamble, the system messages, the
conversation. The system and developer roles go into the second block, and
everything else into the third. A request that carries no user or assistant
message is refused, because the third block would be empty.

## Do's and Don'ts

### Do:

- **Do** echo the model the client asked for, suffix included.
- **Do** keep `type` and `code` equal in the error object.
- **Do** pass the text of the output file on unchanged.
- **Do** name the refused fields in the refusal.
- **Do** keep the identifier prefixes `chatcmpl-`, `resp_` and `msg_`.

### Don't:

- **Don't** wrap the answer in an envelope of your own.
- **Don't** trim the answer, or reformat it, or explain it.
- **Don't** accept a field the gateway cannot honour. Silence there is
  worse than a 400.
- **Don't** add a colour, a style or an image to a response.
- **Don't** change the shape of the prompt. The CLI reads it as it stands.

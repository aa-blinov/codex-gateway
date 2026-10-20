# Documentation

The documents in this folder describe how `codex-gateway` is put together.
There are two of them, and neither repeats the README.

| Document | What it covers |
| --- | --- |
| [architecture.md](architecture.md) | The parts of the server, the data flow through them, the decisions that are not obvious and the boundary around the subprocess. |
| [screenshots/README.md](screenshots/README.md) | Why the folder is empty, and what to put in it if the repository ever grows a picture. |
| [../README.md](../README.md) | The front door: the routes, the flags and how to run the gateway. |
| [../PRODUCT.md](../PRODUCT.md) | The product record: the users, the purpose and the capabilities the code confirms. |

## Reading order

New here: [../README.md](../README.md), then
[architecture.md](architecture.md). The README gives you the commands and
the routes, the architecture gives you the path a request takes through
them.

Changing the code: [architecture.md](architecture.md), then
[../AGENTS.md](../AGENTS.md) for the traps. Several of the traps are the
places where the obvious implementation is the wrong one, and they are the
first thing a change to this server is likely to run into.

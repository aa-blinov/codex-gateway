# Screenshots

The repository has no screenshots, and it is not missing any.

`codex-gateway` is an HTTP server. It has no page, no window, no image file
and no asset of any kind in the tree. What a person sees is the JSON that a
client renders, and a client the gateway does not ship is not something a
screenshot could show.

The screenshots folder stays in the tree because the house standard keeps a
place for it in every repository, and because the README has a section that
would carry images if the project ever grew any.

If a picture is added later, it goes in this folder and it is referenced
from the README by a relative path:

```markdown
![A client pointed at the gateway](docs/screenshots/client.png)
```

A relative path resolves both on GitHub and in a local checkout, and the
house-style checker verifies that every image reference points at a file
that exists, so a renamed picture cannot quietly break the README.

# README screenshots

The images in this folder are the committed tour a reader sees before running
anything — not throwaway visual-QA captures. Regenerate them when the UI
changes, and keep the set small and current.

## Regenerate

```sh
make up        # or: make dev-up  — the UI answers on http://localhost:5173
make screenshots
```

`make screenshots` refuses to run when Node or `web/node_modules` is missing, or
when the app is not answering, and the harness itself refuses to write a capture
that showed a console error, a page error, or almost no text. A blank or broken
image is never committed.

One-time setup (Playwright is a dev dependency; the browser is not):

```sh
cd web && npx playwright install chromium
```

## What is captured

| File | Page |
| --- | --- |
| `catalogue-dark.png`, `catalogue-light.png` | the public catalogue (`/`) |
| `login-dark.png`, `login-light.png` | the login form (`/login`) |
| `signup-dark.png`, `signup-light.png` | the sign-up form (`/signup`) |

`/files` is session-gated and is not in the set: capturing it reproducibly needs
a seeded account, which is a separate decision.

## Gotchas

- **No fonts, no text.** Chromium renders glyphs it cannot find as nothing, so
  every PNG comes out empty. The harness catches this as "almost no visible
  text" and refuses the capture; install fonts in the capture environment rather
  than raising the threshold.
- **The API must be up.** The catalogue fetches `/v1/templates`; with the API
  down the page renders an error state and the capture is refused or wrong.
- **The database is not seeded.** A fresh stack shows the catalogue's empty
  state — that is what the image shows, and it is honest until a seed script
  exists.

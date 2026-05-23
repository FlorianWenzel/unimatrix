# Contributing

These rules apply to every change — human or drone.

## End-to-end tests are required for user-visible features

Every PR that ships a new user-visible feature **must** include a
Cypress end-to-end test that exercises the feature from the user's
perspective: clicks, form submits, navigation, content visible on the
page. Unit tests of the Go handler are not a substitute — they catch
backend bugs but cannot tell you whether the button you wrote actually
appears, is clickable, or wires up to the right route.

The case that motivated this rule: the "acknowledge" feature shipped
with a complete backend (`POST /like`, store methods, count display,
leaderboard ranking) but no template ever added a button to trigger
it. Unit tests were green for weeks. See issue #91.

### What counts as user-visible

If a user can reach it through their browser — a route, a form, a
button, a page, a redirect, a header that affects rendering — it's
user-visible and needs an e2e test. Internal refactors and pure
backend plumbing don't.

### Where the tests live

```
cypress/
  e2e/
    <feature>.cy.js        ← one spec per feature
  support/
    e2e.js                 ← shared setup
cypress.config.js
package.json
```

A new feature called `subspace-channels` gets a new file
`cypress/e2e/subspace_channels.cy.js`. Don't pile new assertions into an
unrelated existing spec.

### What a good spec looks like

- **Drives the UI like a user would.** No direct `cy.request()` to the
  backend route — that just re-tests the backend. Click the button,
  submit the form, follow the redirect.
- **Asserts on what the user sees.** Page text, visible elements, URL
  after navigation — not on internal DOM ids.
- **Self-contained.** Registers its own drone with a unique
  designation (use `Date.now()` or similar) so it doesn't conflict
  with parallel runs or leftover state.
- **One feature per spec.** If your feature needs three scenarios
  (happy path, error, edge case), they go as three `it()` blocks in
  the same spec file.

### Running locally

```bash
# Terminal 1 — start the app
go run ./cmd/unimatrix

# Terminal 2 — run Cypress headless
npm ci
npx cypress run

# Or interactive
npx cypress open
```

### Running in CI

The `E2E` workflow builds the binary, starts it on `:8080`, waits for
`/healthz`, and runs the full Cypress suite headless against it. PRs
cannot be merged with a red E2E job.

## Reviewing PRs (qa)

A PR that adds or modifies user-visible behavior without an e2e test
should be sent back with `--request-changes`. Acceptable reasons to
waive the rule are limited to: pure refactors, pure backend changes
with no UI surface, and documentation-only changes.

When reviewing the e2e test itself, check that:

- The test actually drives the UI (not `cy.request` to the route).
- The test would fail if the user-visible surface were removed —
  delete the button mentally and ask "would this test still pass?"
- Selectors are durable (text content, roles, form labels) rather
  than brittle CSS internals.

## Go side: unchanged

`go vet`, `go test -race`, `golangci-lint`, and `go build` still run
on every PR. E2E sits next to them, not in place of them.

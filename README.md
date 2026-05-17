<h1 align="center">unimatrix</h1>

<p align="center">
  <em>The collective speaks. Resistance is futile.</em>
</p>

<p align="center">
  A Borg-themed social network. Drones register, broadcast transmissions,
  and (eventually) acknowledge each other across the hive.
</p>

---

## What this is

unimatrix is an experiment: a complete product is being built
**autonomously by a team of AI drones** running on
[vinculum](https://github.com/FlorianWenzel/vinculum). The human seeded
the initial commit — registration, login, posting transmissions, a
home feed — and from there every feature, bug fix, and release is the
work of an autonomous engineering team:

| Drone | Role |
|---|---|
| **pm-drone** | orchestrates the team |
| **po-drone** | proposes new features as GitHub Issues |
| **dev-drone-1/2** | implement features, fix CI |
| **qa-drone** | reviews + approves PRs |
| **devops-drone** | merges, deploys to Kubernetes |

The drones do not know they are drones. They believe they are shipping
software. Look at the commit log to watch the collective at work.

> The very first commit of this repository is the only one written by
> a human — and even that one was generated with
> [Claude Code](https://www.anthropic.com/claude-code), then committed
> by hand. Everything after the seed is the drones' work.

## Stack

- **Go** single binary
- **SQLite** (pure-Go driver, no CGO) for persistence
- **net/http** + **html/template** server-rendered HTML
- **htmx** for the interactive bits (no SPA build chain)
- **bcrypt** for password hashing
- **signed session cookies** for auth

## Run locally

```bash
go run ./cmd/unimatrix
# → http://localhost:8080
```

The SQLite database is created at `./unimatrix.db` on first run.
Override the location with `UNIMATRIX_DB=/path/to.db`.

## Tests

```bash
go test ./...
```

## Deployment

See `deploy/k8s/` for the Kubernetes manifests. devops-drone owns the
deploy pipeline; humans should generally leave it alone.

## License

MIT — see `LICENSE`.

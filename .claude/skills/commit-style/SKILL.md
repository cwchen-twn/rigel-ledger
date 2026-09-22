---
name: commit-style
description: Writes commit messages for the rigel-ledger repo in its conventional-commit style - a subject that states the finding rather than the edit, and a prose body that records the mechanism, the measurement and what was rejected. Use when committing, staging and committing, writing or rewriting a commit message, or preparing a PR description in this repository.
---

# Commit messages (rigel-ledger)

Modelled on the hcloud repo's convention and adapted to this one. As of 2026-09-22,
this repo's history has 48 non-merge commits, and most of them predate the convention:

- 5 of the 48 have a body.
- 27 subjects open with a past-tense label ("Added", "Updated", "Fixed").
- 14 are `wip:`.

**Do not imitate the history. Follow this file.** The older commits show what *not* to do.

## Format

```
<type>: <subject>

<body>

Co-Authored-By: <the attribution line the session gives you>
```

- **No scope.** Write `feat:`, never `feat(models):`. None of the 48 have one.
- **No trailing period.** One old commit has one; do not add another.
- Wrap the body at **~72-79 columns**.
- Use the `Co-Authored-By:` trailer the current session supplies. Do not copy an old
  one from `git log`, because the model name changes.

## Types

| Type | Use for | Historical count |
|---|---|---|
| `feat` | new capability: an endpoint, a page, a migration that adds a domain, an importer | 8 |
| `fix` | wrong behaviour, a broken trigger, a claim corrected | 3 |
| `chore` | dependency and Go bumps, renames, tooling, CI, lockfiles | 20 |
| `docs` | documentation only: README, CLAUDE.md, `docs/`, skills | 1 |
| `wip` | **do not use** | 14 |

`refactor` and `perf` are allowed when the change is purely that. A code change that
also updates its docs is still `feat:` or `fix:`, not `docs:`.

## The subject states the finding, not the edit

Aim for **50-80 characters**. The historical median is 44, which is too thin. Use the
imperative or a plain statement, never a past-tense label. A comma clause that attaches
the *why* is encouraged.

**Good:**

```
chore: Bump x/text to v0.42.0, because govulncheck traced pgx into norm.*
fix: The balance check summed raw amounts, so every FX journal was rejected
feat: Books with members, so a family can share one set of accounts
docs: Record why the 4-grade Taiwan business chart of accounts was dropped
```

**Too thin (this is how the old history reads):**

```
chore: Updated dependencies
wip: Implementing ledger page and transaction page
fix: Fixed issues discovered by static scans
```

## The body is not optional

Every commit gets a body, except a pure lockfile `chore:`. Write prose paragraphs, and use
bullets only for enumerations inside the prose. Roughly in this order:

1. **What was wrong or missing**, with the mechanism: which trigger, which query,
   which handler.
2. **The measurement.** Paste what you actually observed: the govulncheck ID and the
   fixed-in version, the `make audit` result, test counts, binary size from
   `make build/compare`, row counts from a migration.
3. **What was believed and turned out false**, in CAPS if it is the load-bearing line.
4. **What was considered and rejected**, and why.
5. **What the operator must now do**: a new migration to apply, a new env var in
   `.env.example`, a `make swag` rerun, or a chart value in hcloud.

### Body conventions

- **ASCII punctuation**: `--` for a dash, `->` for an arrow. No `—` or `→`.
- **CAPS** for the one claim a future reader must not miss.
- Bullets are two-space indented `*` or `-`.
- Name migrations by number (`000001`) and files by repo-relative path.
- Financial examples use exact decimals, never rounded floats.

### A body in the house style

```
chore: Bump x/text to v0.42.0, because govulncheck traced pgx into norm.*

make audit failed after the Go 1.26.8 bump on GO-2026-5970 (infinite loop
on invalid input in golang.org/x/text, fixed in v0.39.0). The traces run
from models.User.UpdateLastLogin -> sql.DB.Exec -> norm.Form.Transform, so
the vulnerable path is reached through the pgx driver, not through our
own use of x/text in internal/response/templates.go.

x/text goes to v0.42.0 (latest). x/crypto goes from v0.52.0 to v0.57.0 at
the same time, for GO-2026-6355/6354/6303: govulncheck did not find them
called, but bcrypt lives in that module and the bump is free. x/mod, x/sync
and x/tools move with tidy.

govulncheck now reports 0 called vulnerabilities and one uncalled
(GO-2026-5932 in x/crypto, with NO FIXED VERSION yet). make audit passes.

Co-Authored-By: <session attribution>
```

## Repo-specific rules

- **Commit on a branch.** `.pre-commit-config.yaml` runs `no-commit-to-branch main`.
  Create a branch first (`git switch -c <short-topic>`).
- **pre-commit runs `make audit` and gitleaks.** Fix the failure, then commit again.
  Never `--no-verify`, and never paste a secret from `.env` into a message.
- **`api/` is generated.** Commit it only as the output of `make swag`, together with the
  handler change that caused it.
- **Migrations**: while the schema is being reset (see `docs/ARCHITECTURE.md`), say in
  the body whether a commit edits `000001` in place or adds a new pair. Once the app is
  deployed, never edit an applied migration.
- The remote is Gitea (`git.chenantunez.com`). Reference issues and PRs as `#12`.
- **PR descriptions**: use the commit subject as the title and the commit body logic
  as the description, plus a "How verified" paragraph.

## Do not

- Use `wip:`, a scope, or a trailing period.
- Write a past-tense label subject ("Added X", "Updated Y").
- Commit with no body, except a pure lockfile `chore:`.
- Use `—` or `→` in the body.
- Commit on `main`, or bypass hooks.
- Claim a measurement you did not take. The whole value of the body is that its numbers
  can be trusted later.

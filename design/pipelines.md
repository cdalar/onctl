# onctl pipelines — design doc

Status: draft v2 (separated infra + deploy commands, env model)
Date: 2026-05-11

## Wedge

**One binary that owns provisioning AND deployment** for an app, from a repo with a Dockerfile + cloud credentials, ending with a printed URL.

Differentiators:

- vs Terraform + Ansible: one tool, one YAML, no HCL/Python split.
- vs Dagger / GH Actions: owns the infra side too.
- vs Coolify / Dokku / CapRover: no pre-existing server — provisions it.
- vs raw `onctl up` + bash: persistent infra, idempotent re-deploy, multi-VM dependency graph, multi-env, URL output.

## North-star demo (60 seconds)

```
$ cd ~/code/my-app                  # has a Dockerfile, EXPOSE 3000
$ export HCLOUD_TOKEN=...
$ onctl deploy
→ no environments found — creating preview env
→ provisioning (hetzner, cx22, nbg1)... up in 38s
→ deploying...
  ✓ upload  (12 MB)
  ✓ build   docker image
  ✓ run     container on :80
✓ preview live at http://5.75.142.18  (auto-destroys in 24h)
```

Second invocation, same env:

```
$ onctl deploy
→ deploying to preview (web@5.75.142.18)
✓ live at http://5.75.142.18  (11s)
```

Promoting to production later:

```
$ onctl infra up --env production
→ creating infrastructure for production... up in 41s
$ onctl deploy --env production
✓ live at http://65.21.7.42
```

## Mental model

Two distinct commands for two distinct lifecycles:

| Command | Lifecycle | Re-run behavior |
|---|---|---|
| `onctl infra ...` | persistent, rare | If named VMs exist, reuse. Else create. **No diff/drift management in MVP.** |
| `onctl deploy` | idempotent, frequent | Always re-execute deployment steps. |

`onctl deploy` auto-runs `infra up` only for the implicit `preview` env. For any named env, missing infra prompts. For `production`, prompts with extra emphasis. This protects against `--env prdo` silently spinning up parallel prod.

Tear-down is always explicit (`onctl infra destroy --env <name>`), except preview which has optional auto-destroy.

## CLI surface

```
onctl infra up        [--env <name>]      # provision (or no-op if exists)
onctl infra destroy   [--env <name>]      # tear down
onctl infra status    [--env <name>]      # show VMs + IPs for an env
onctl infra list                          # list all known envs (declared + provisioned)

onctl deploy          [--env <name>]      # deploy app; auto-runs infra for preview only
onctl deploy --plan   [--env <name>]      # resolve plan, print, no execution
onctl deploy --logs   [--env <name>]      # stream/replay last deploy logs
onctl deploy -f path.yml [--env <name>]   # explicit pipeline file

onctl preview reap                        # destroy expired preview envs
```

`onctl up` (the existing ad-hoc single-VM command) stays unchanged and coexists. Consider renaming to `onctl vm up` later to free `up` for pipeline-level use.

### `--env` resolution rules

1. `--env <name>` explicit → use it.
2. No `--env`, `default_env:` set in yaml → use the default.
3. No `--env`, no default, zero existing envs → use implicit `preview`.
4. No `--env`, no default, exactly one existing env → use it.
5. No `--env`, no default, multiple existing envs → error: "ambiguous; pass --env or set default_env".

### Auto-provision rules for `onctl deploy`

| Env name | Infra missing → behavior |
|---|---|
| `preview` (implicit or named) | Silently provision |
| Any other name | Prompt "infra for `<name>` doesn't exist; create now? (y/N)" |
| `production` | Same prompt, but require typing the env name to confirm |

## YAML shape

### Minimal explicit form

```yaml
name: my-app
version: 1

infrastructure:
  provider: hetzner          # multi-cloud schema; only hetzner driver in MVP
  vms:
    web:
      type: cx22
      location: nbg1
      cloud_init: ./cloud-init/docker.yaml

deployment:
  steps:
    - target: web
      upload: { src: ., dest: /opt/app, exclude: [.git, node_modules] }
    - target: web
      run: |
        cd /opt/app
        docker build -t app .
        docker rm -f app || true
        docker run -d --name app --restart=always -p 80:3000 app

endpoint: http://${{ vms.web.public_ip }}
```

### With environments

```yaml
name: my-app
version: 1
default_env: staging        # optional; otherwise the rules above apply

infrastructure:             # baseline; envs override
  provider: hetzner
  vms:
    web:
      type: cx22
      location: nbg1
      cloud_init: ./cloud-init/docker.yaml

deployment:
  steps:
    - target: web
      upload: { src: ., dest: /opt/app }
    - target: web
      run: cd /opt/app && docker build -t app . && docker rm -f app || true && docker run -d --name app -p 80:3000 --restart=always app

endpoint: http://${{ vms.web.public_ip }}

environments:
  preview:                  # also synthesized at runtime if not declared
    auto_destroy_after: 24h
    vms:
      web: { type: cx22 }

  staging:
    vms:
      web: { type: cx22 }
    env:
      APP_ENV: staging

  production:
    vms:
      web:
        type: cx32
      db:
        type: cx32
        cloud_init: ./cloud-init/postgres.yaml
    env:
      APP_ENV: production
    endpoint: https://app.example.com
```

### Multi-VM with dependency graph

```yaml
deployment:
  jobs:
    schema:
      target: db
      steps:
        - upload: { src: ./sql/schema.sql, dest: /tmp/schema.sql }
        - run: psql -U app -f /tmp/schema.sql
    app:
      target: web
      needs: [schema]
      env:
        DATABASE_URL: postgres://app:${{ secrets.DB_PASSWORD }}@${{ vms.db.private_ip }}/app
      steps:
        - upload: { src: ., dest: /opt/app }
        - run: cd /opt/app && docker compose up -d --build
```

Either `deployment.steps:` (flat sequential) or `deployment.jobs:` (graph). Not both.

### Environment override semantics

An `environments.<name>` block deep-merges over the top-level `infrastructure` and can override `endpoint`. Anything not set inherits from the baseline. The implicit `preview` env is equivalent to `{ vms: <baseline>, auto_destroy_after: 24h }` if not explicitly declared.

### Step kinds (MVP)

- `run: <shell>` — exec over SSH on `target` VM
- `upload: { src, dest, exclude?: [] }` — rsync local → remote
- `download: { src, dest }` — rsync remote → local

Add later: `wait_for`, `if:`, etc.

### Expression context

- `env.name` — current environment name
- `vms.<name>.public_ip` / `.private_ip` / `.id`
- `secrets.<NAME>` — declared at top level, values from env or `--secret-file`
- `vars.<name>` — declared at top level
- `needs.<job>.outputs.<key>` — post-MVP

Lib: `expr-lang/expr`.

## Zero-config path

If `onctl deploy` runs with no yaml file:

1. Look for `Dockerfile` in cwd. If absent, error with clear message.
2. Parse `EXPOSE` to find app port (default 3000).
3. Synthesize an in-memory pipeline equivalent to the minimal explicit form (single cx22 VM, docker cloud-init, upload + build + run), with `preview` env applied.
4. Execute it.
5. On success, write `onctl-deploy.yml` to cwd with a `# auto-generated, edit freely` header so the user can customize from there.

Same YAML format hand-written or auto-generated — no two formats.

## Execution model

### Environment identity

- Real cloud resources are tagged/named with `<vm>-<app>-<env>-<short-hash>`, e.g. `web-myapp-production-a3f`. Short hash derived from `name + git remote url` so two repos with same name don't collide.
- Cloud-side tags: `onctl:app=<name>`, `onctl:env=<env>`, `onctl:preview=true|false`. Enables `onctl preview reap`.

### Provisioning (`onctl infra up`)

For each VM in the merged `infrastructure` for the target env:

1. Check cloud for a VM with the computed name.
2. If exists: fetch IPs, continue.
3. If not: create + wait for cloud-init.
4. **No drift detection in MVP.** Changed `type:` or `cloud_init:` will not recreate; output warns: "infra exists; ignoring changed fields X, Y — run `onctl infra destroy --env <name>` first to apply."

### Deployment (`onctl deploy`)

- Resolve target env (rules above).
- If infra missing, follow auto-provision rules.
- Execute `deployment.steps` sequentially, or `deployment.jobs` via topo sort + parallel.
- Stream stdout/stderr with per-step prefix.
- On any step failure: stop, exit non-zero, leave infra running.
- Print resolved `endpoint:` on success.
- Persist logs to `~/.onctl/runs/<app>-<env>/<timestamp>/`.

### State

Minimal — designed for "no diffing":

```
~/.onctl/runs/<app>-<env>/
  last.json           # last successful: vm names, IPs, endpoint URL, timestamp
  <timestamp>/
    deploy.log
    plan.yml          # the resolved plan executed
```

`last.json` is a cache. Truth lives in the cloud; rebuildable from `onctl infra status`.

### Preview lifecycle

- Tagged `onctl:preview=true` and `onctl:expires_at=<iso8601>`.
- `onctl preview reap` lists previews past expiry and destroys them after confirmation (`--yes` for automation).
- No background daemon — reaper is user-invoked (or cron'd by the user).

### CI compatibility

- Pure CLI, no daemon, no remote backend.
- Reads creds from env (`HCLOUD_TOKEN`, etc.).
- `--plan` for dry-run in PR checks.
- Common GH Actions pattern: PR opens → `onctl deploy --env preview-pr-${{ pr.number }}` → comment URL → on PR close, `onctl infra destroy --env preview-pr-${{ pr.number }}`.
- Race condition: two concurrent runs against same env race on "exists?" check. MVP accepts; document "one operation per env at a time." File-based locking post-MVP.

### Secrets

- Names declared under top-level `secrets: [DB_PASSWORD, ...]`.
- Values from env var (default) or `--secret-file env-style.env`.
- Per-env overrides via `environments.<name>.secrets:` (different secret-file path or just allowed names).
- Redacted in console + persisted logs (substring replace on write).
- Never written to `last.json` or `plan.yml`.

## Implementation sketch

```
internal/pipeline/
  schema.go        # YAML structs + validation
  envresolve.go    # --env rules, env merging, auto-provision rules
  expr.go          # interpolation (expr-lang/expr)
  synthesize.go    # zero-config: Dockerfile → in-memory pipeline
  graph.go         # topo sort for deployment.jobs
  runner.go        # phase orchestration
  state.go         # ~/.onctl/runs/ read/write
  secrets.go       # resolution + redaction
  preview.go       # tagging, expiry, reap
cmd/infra.go       # `onctl infra` subcommands
cmd/deploy.go      # `onctl deploy`
cmd/preview.go     # `onctl preview reap`
```

Reuses `internal/cloud/*`, cloud-init helpers, SSH machinery (likely needs a thin abstraction over today's `runContainer`).

## Open questions

1. **Where does `onctl-deploy.yml` live?** Top level (visible, matches `Dockerfile`) or `.onctl/deploy.yml` (hidden, matches `.github/workflows/`). Lean top level.
2. **Bundled `docker on Ubuntu` cloud-init for zero-config.** `embed.FS` in the binary, or fetched from a templates repo? Ties into the template-marketplace direction — if marketplace is months away, embed for now.
3. **Helper step types.** Should we provide `docker_run:` / `compose_up:` step kinds that handle the stop+remove+restart dance, so users can't write a non-idempotent `run:` by accident?
4. **Preview env naming.** Single `preview` env, or auto-generated names like `preview-3f2`? Single is simpler; auto-generated supports multiple parallel previews (useful for PR-per-preview workflows).
5. **`infra status` output format.** Plain text default, with `--json` for scripting? Probably yes.

## Milestones

- **M0 — schema + parser**: structs, validation, env merge logic, `deploy --plan`. No execution.
- **M1 — single-VM happy path**: `onctl infra up` + `onctl deploy` with explicit yaml, one VM, sequential steps, prints endpoint. Hetzner only. No env support (single implicit env).
- **M2 — zero-config preview**: Dockerfile-only → synthesized pipeline → preview env → URL. The demo works end-to-end.
- **M3 — environments**: `environments:` block, `--env` flag, resolution rules, auto-provision rules, prod confirmation.
- **M4 — multi-VM + DAG**: `deployment.jobs` with `needs:`, parallel execution, cross-VM interpolation.
- **M5 — ops + preview lifecycle**: `infra destroy/status/list`, `deploy --logs`, secret handling + redaction, `preview reap`, expiry tagging.
- **M6 — CI hardening**: race messaging, exit codes documented, GH Actions PR-preview example in README.

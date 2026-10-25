# onctl claude: prebuilt claude-agent image on ac, and a fresh VM per run

> Plan written 2026-10-25 in a Claude Code session; PR 1 became the move of the image system to boxctl-io/boxctl-images (ghcr.io/boxctl-io/claude-agent), PR A is cdalar/onctl#1234, PR B is the branch this file lands with.

## Context

`onctl -p ac claude` works today but every new Apple container pays a slow
first boot: the ac bootstrap apt-installs sshd and sudo into stock
`ubuntu:24.04`, then `claudeSetupScript` apt-installs tmux/git/curl and
downloads Claude Code. boxctl already has an image with all of that baked
in (`images/claude-agent` in boxctl-vms: debian-slim + git, gh, tmux, curl,
rsync, jq, ripgrep, sshd, sudo, the GitHub forwarding helpers, and Claude
Code in /root/.local/bin), but it is only built as a linux/amd64 ext4 for
Firecracker and published to R2. No OCI image of it exists on any registry
(ghcr.io/cdalar/* and ghcr.io/boxctl-io/* return 404), and this Mac is arm64.

Separately, `onctl claude` still uses boxctl's *old* model: one long-lived
`claude-<project>` box, project copied once, reused forever. boxctl switched
on 2026-10-14 to a fresh box per run (cloned from origin on a new branch,
work fetched back as `box/*`, box deleted when Claude exits; `--keep`
restores the old model). onctl's `--repo` code even says "without its goals
and ephemeral boxes". The user wants onctl to match boxctl.

The two fit together: ephemeral only feels free when a fresh VM boots
ready in seconds, which is what the prebuilt image gives ac.

Decisions taken with the user:
- Image reaches ac by publishing a multi-arch OCI image to ghcr from
  boxctl-vms CI.
- Ephemeral becomes the default for every provider (boxes included),
  matching boxctl.

Delivered as three PRs, in this order (each verifiable on its own):

1. **boxctl-vms**: publish `ghcr.io/cdalar/claude-agent` (amd64 + arm64).
2. **onctl PR A**: `onctl -p ac claude` defaults to that image; GitHub
   forwarding allowed when the VM has the image's helpers.
3. **onctl PR B**: fresh VM per run, `--keep` for the old behaviour.

---

## PR 1 — boxctl-vms: publish claude-agent to ghcr (multi-arch)

Repo: `/Users/cd/cdalar/boxctl-vms` (branch off main).

Facts checked:
- `images/claude-agent/Dockerfile` is `FROM debian-slim` and debian-slim is
  `FROM debian:trixie-slim@sha256:a99cfc…` with `RUN --mount=from=common`.
  That digest is a multi-arch **index** (amd64, arm64, …), so an arm64
  build needs no Dockerfile change. `gh` exists for trixie arm64. The
  Claude installer picks the arch at build time.
- `cmd/boxctl-image/build.go` already generates a buildx **bake** file
  (`bakeFile`) wiring `debian-slim` as a target and `common` as a named
  context, hard-coded to `linux/amd64` (`buildPlatform`). It is written to
  a temp dir and consumed internally; nothing prints it.
- `.github/workflows/build-image.yml` is the existing ghcr publish pattern
  (login-action, metadata-action, `permissions: packages: write`,
  `runs-on: onctl-2`).

Changes:
1. `cmd/boxctl-image/build.go` + `main.go`: add `-bake-only <path>` to
   `boxctl-image build`: write the generated bake JSON there and stop
   (sits beside `-tar-only`; reuse `bakeFile`). Unit test: the file names
   the `claude-agent` target with `debian-slim` as `target:` context and
   `common` as a directory context.
2. New workflow `.github/workflows/publish-oci-images.yml`:
   - `on: push: branches: [main]` with `paths: images/claude-agent/**,
     images/debian-slim/**, images/common/**, cmd/boxctl-image/**`, plus
     `workflow_dispatch`.
   - steps: checkout, setup-go, setup-qemu-action (arm64 on the amd64
     runner), setup-buildx-action, login to ghcr with `GITHUB_TOKEN`,
     `go run ./cmd/boxctl-image build claude-agent -bake-only $RUNNER_TEMP/bake.json`,
     then
     `docker buildx bake -f $RUNNER_TEMP/bake.json <target> --push
       --set '*.platform=linux/amd64,linux/arm64'
       --set '<target>.tags=ghcr.io/cdalar/claude-agent:latest'
       --set '<target>.tags=ghcr.io/cdalar/claude-agent:sha-<short>'`
     (`<target>` is `bakeTargetName("claude-agent")`; check what it maps to).
   - Header comment: why this exists (onctl's ac provider runs it as an
     Apple container), that the ext4 publish in build-fc-images.yml is
     unchanged, and that the image is public so `container run` pulls it
     anonymously.
3. `images/claude-agent/Dockerfile` header comment: one line noting it is
   also published as an OCI image for onctl's ac provider.
4. After merge: make the ghcr package **public** (one-time, in GitHub
   package settings) and check
   `curl -s https://ghcr.io/token?scope=repository:cdalar/claude-agent:pull`
   + manifest fetch returns an index with both platforms.

Risk: the installer's `claude --version` runs under qemu-user for arm64;
expected to work but slow. Fallback if it fails: run that `RUN` only
natively by moving the arm64 build to an arm64 runner, or build with
`--platform` native on this Mac once and push by hand.

---

## PR 2 — onctl PR A: ac uses the prebuilt image

Repo: onctl worktree `/Users/cd/.herdr/worktrees/onctl/worktree-clear-stone-e8b2`
(branch off main).

Key code: `cmd/claude.go` `runClaude` lines 165–186 (the `!onBoxes()`
block), `cmd/claudevm.go` `onBoxes`/`ensureClaudeVM`, `pkg/cloud/ac.go`
`acBootstrap` (already skips apt when sshd and sudo exist; runs
`ssh-keygen -A`, so the image's deliberately missing host keys are fine;
`--entrypoint /bin/sh` bypasses systemd, which is fine).

Changes:
1. `cmd/claude.go`: const `acClaudeImage = "ghcr.io/cdalar/claude-agent:latest"`
   with a comment (what it has, why only ac gets it: the other VM
   providers take cloud images, not OCI). In the `!onBoxes()` block,
   replace the unconditional `claudeImage = ""` with: unchanged flag and
   provider is ac → `acClaudeImage`; unchanged flag otherwise → `""`.
   Add `onAC()` next to `onBoxes()` in `claudevm.go`
   (`provider.(*cloud.ProviderAC)`; root.go builds it as a pointer).
   `runRepo` is called after this block and reads the same var, so
   `--repo` on ac gets the image too.
2. GitHub forwarding on a VM that has the helpers: today the block forces
   `--github off` on any non-boxes provider. Change to a runtime check
   after ssh is up (in `runClaude`/`runRepo`, after `openClaudeBox`):
   if `claudeGitHub == githubForward` and not on boxes, run
   `test -x /usr/local/bin/git-credential-boxctl` on the box; keep forward
   if it passes, else fall back to off with the existing message. The
   socket path matches already (`/root/.boxctl/gh.sock`,
   `BOXCTL_GH_SOCKET`), and the image's `boxctl-forwarding.conf` sets
   `StreamLocalBindUnlink`. Keep the early error for an explicit
   `--github forward` only when the check fails.
3. Comments: `claudeInstall`'s "rather than baking Claude Code into the
   image" is stale (boxctl bakes it now) — fix the sentence where touched.
4. `internal/files/init/onctl.yaml` ac section: a comment line that
   `onctl claude` uses `ghcr.io/cdalar/claude-agent` on ac unless
   `--image` is given; `ac.image` stays the default for `onctl create`
   (memory: config fixes must land in the init template).
5. README line 22 / the `onctl claude` help Long: mention ac boots from the
   prebuilt image.

Tests (`cmd/claude_test.go`, `cmd/claudevm_test.go` patterns; set
`provider` to `&cloud.ProviderAC{}` / `&cloud.ProviderHetzner{}` / boxes):
- image resolution: ac + flag unchanged → ghcr ref; ac + `--image x` → x;
  hetzner + unchanged → ""; boxes → `claude-agent`.
- the helper check script string and the forward/off decision from its
  exit code (pure function taking `hasHelpers bool, explicit bool`).

---

## PR 3 — onctl PR B: fresh VM per run (port of boxctl's ephemeral mode)

Source of truth for the port: boxctl `cmd/claudeephemeral.go`,
`cmd/boxname.go`, `cmd/claude.go` (`openSession`, `startSession`,
`prepareSession`), `cmd/claudecopy.go` (`cloneURL`, `checkCopy`),
`cmd/claudegit.go` (`fetchBox`). onctl already has the building blocks:
`openClaudeBox`, `checkProject`/`markProject`, `copyProject`,
`cloneScript` (clauderepo.go), `claudeFetch` (claudegit.go),
`startTokenServer` + `attach(... -R ...)`, `listClaudeVMs`,
`sessionRunning`, `provider.Destroy`, `newClient().Destroy`.

### Behaviour (matches boxctl, minus reconnect, labels, goals)

- `onctl claude` (no `--keep`, `--box`, `--detach`, `--task`,
  `--own-box`): new VM `<project>-<slug>-<id>` (slug from `--prompt`, else
  adjective-noun; id 4 hex; 40-char cap), project put there, Claude runs,
  and when Claude **exits** the VM's branches and uncommitted work are
  fetched here as `box/*` and the VM is destroyed. Detaching with Claude
  still running keeps it, with a hint to come back with `--box`. Setup
  failure or Ctrl-C during setup destroys it. If the fetch fails the VM is
  kept with a message saying how to fetch and destroy by hand.
- Project on the VM: a git repo with an `origin` is **cloned** from it
  (GitHub origins normalised to https) onto branch `onctl/<slug>-<id>`;
  anything else is copied as today. Local uncommitted work does not go.
  `--keep`'s new box clones too, on `onctl/<project>`.
- `--keep` / `-k`: today's behaviour (long-lived `claude-<project>`).
  `--box`, `--detach`, `--task`, `--own-box` imply it.
- `--repo` keeps today's semantics in this PR (own box, not deleted).
  boxctl's "delete once the work is on GitHub" is a listed follow-up.
- No reconnect-on-drop (boxctl's claudereconnect.go): out of scope; a
  dropped connection leaves the VM, as today.

### Files

1. **New `cmd/boxname.go`** (+ `boxname_test.go`): port `boxName`
   (`box()`, `branch()` with `onctl/` prefix), `nameSlug`, `textSlug`,
   `newNameID`, `generateName`, the adjective/noun/stop-word lists, and
   `newBoxName(ctx, prefix, text)` taking existing names from
   `listClaudeVMs(ctx)` (works for boxes and every provider). Port
   `TestTextSlug` and `TestGenerateName`.
2. **New `cmd/claudeephemeral.go`**: `claudeKeep` flag var,
   `ephemeralRun()`, `ephemeralBoxName`, `saveBoxWork`, `discardBox`
   (boxes → `newClient().Destroy`; else `provider.Destroy(cloud.Vm{Name})`),
   and `(*claudeTarget).end()` with its own 2-minute background context.
   Table test for `ephemeralRun`.
3. **`cmd/claudegit.go`**: split `claudeFetch` into
   `fetchBox(ctx, g *claudeGitTarget) error` (snapshot + one fetch +
   report) so `saveBoxWork` reuses it; `claudeFetch` becomes open +
   `fetchBox`. No behaviour change for `onctl claude fetch`.
4. **`cmd/clauderepo.go` → `cmd/claudecopy.go`**: move `cloneScript`
   next to `copyProject`, add boxctl's "already exists" guard, add
   `cloneURL(dir)` (reuse `githubRepo` from claudegithub.go). `TestCloneScript`
   gains the guard case; new `TestCloneURL`.
5. **`cmd/claude.go`**:
   - flag `--keep/-k`; Long help rewritten after boxctl's (fresh VM per
     run; what is cloned vs copied; `--keep`; examples) — note boxctl's
     text still says detach deletes, which is stale; write the correct
     behaviour.
   - `claudeTarget` gains `ephemeral bool`; `openClaudeBox` unchanged.
   - `runClaude`: `signal.NotifyContext(SIGINT, SIGTERM)` so Ctrl-C during
     setup reaches `end()`. After `resolveTask()`: if `ephemeralRun()`,
     `projectDir()`, pick `ephemeralBoxName`, set `claudeBox` and a new
     `claudeBranch`. After `openClaudeBox`: `t.ephemeral = …`,
     `defer t.end()` (replaces `defer t.box.close()`), print boxctl's
     "Ephemeral: … (--keep keeps it)" note. On any setup error after the
     VM exists, `end()` discards it (needsCopy still true → nothing to
     fetch). If `openClaudeBox` itself fails after creating, call
     `discardBox(claudeBox)`.
   - project step: `if t.needsCopy { if url := cloneURL(dir); url != "" { clone on branch } else { copyProject } ; markProject }`.
     Branch: `claudeBranch` for ephemeral, `onctl/<box name minus claude->`
     for a new kept box. Error for a failed clone mirrors `runRepo`'s
     "private repository needs --github store" hint when no token path.
   - GitHub token for the clone: it runs before attach, and onctl's
     forwarding is `ssh -R` at attach time. Add `(*boxSSH).runWith(ctx,
     extra []string, command)` (same as `run` plus extra ssh args and
     `ControlPath=none`) and, when `claudeGitHub == githubForward`, run
     the clone through it with `startTokenServer` and
     `-R <sess.ghSocket>:<srv.socket>` (same pattern as `attachClaude`,
     factor the socket cleanup into a helper). `--github store` works as
     now (token stored before the clone — move `storeGitHubToken` before
     the project step, as boxctl does).
   - `startSession`: after `attachClaude` returns nil and `t.ephemeral`,
     check `sessionRunning` (30 s background ctx); if running, set
     `t.ephemeral = false` and print "still running … kept -- attach with
     <hint>". onctl's `commandLine` already `exec`s claude, so the tmux
     session ends when Claude exits (no `shellAfter` port needed).
6. **`cmd/mux.go`** herdr `start`: when the session is ephemeral and not
   detached, the pane is `sh <cmdfile>; herdr server stop` so the attached
   `exec herdr` returns when Claude exits (boxctl mux.go:173–198). Carry an
   `ephemeral bool` on `taskSession` for this. Extend the herdr start test.
7. `cmd/claudels.go`: no change needed (ephemeral VMs show up by name);
   check the output still reads well with `<project>-<slug>-<id>` names.
8. **Docs**: README `onctl claude` bullet and the help text; `docs/plans/`
   copy of this plan.

### Tests
- `boxname_test.go` (ported), `ephemeralRun` table, `cloneURL`,
  `cloneScript` guard, herdr start with/without ephemeral, `runWith` args
  (mirrors `TestAttachArgsGetTheirOwnConnection`), `saveBoxWork` decision
  for needsCopy / non-git (pure part factored out).
- `go test ./cmd/... ./pkg/... ./internal/...` green; `go vet`.

---

## Verification (end to end, on this Mac)

Preconditions: `container system start`; onctl built from the branch
(`go build -o /tmp/onctl .` in the worktree). Note the macOS Local Network
gotcha from memory: run from a GUI terminal, not over ssh, or Go's dial to
192.168.65.x gets "no route to host".

1. PR 1: `container image pull ghcr.io/cdalar/claude-agent:latest` on the
   Mac succeeds and `container image inspect` shows arm64.
2. PR A: `onctl -p ac claude --keep` in a small repo: no "Installing…"
   lines, sshd up in seconds, Claude starts; `onctl -p ac ls` shows the
   container with the ghcr image; `--image ubuntu:24.04` still takes the
   slow path. `git push` inside the session goes through forwarding when
   attached (github.log gets an entry).
3. PR B: `onctl -p ac claude --prompt "add a README line"`: a VM named
   `<repo>-add-readme-line-xxxx` appears, Claude is on branch
   `onctl/add-readme-line-xxxx`; quit Claude → "Fetching…", refs
   `box/onctl/…` and `box/wip` exist locally, VM gone from `onctl -p ac ls`.
   Detach (Ctrl-b d) instead → VM kept with the `--box` hint; `onctl -p ac
   claude --box <name>` reattaches. Ctrl-C during the clone → VM destroyed.
   `onctl claude --keep` → `claude-<repo>` as before. Same smoke run once
   with `-p boxes`.

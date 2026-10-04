# Hosted boxes as an onctl provider

> **Status:** rollout step 1 (provider and login) is built: `-p boxes`,
> `onctl login`/`logout`, `cloud.Dialer` with `tools.Remote.Dial`, and
> `onctl ssh-proxy`. Open questions 1, 2 and 5 were settled as proposed:
> provider ID `boxes`, `onctl login`, `gorilla/websocket`. Step 2 is under
> way: `onctl port-forward` (alias `pf`) is built, for every provider --
> over the ssh connection, so through the tunnel for boxes. `sizes` comes
> next, also for every provider, and `--idle-ttl`. Steps 3-5 remain.

## Context

boxctl.io runs Firecracker microVMs ("boxes") for people: a hosted control
plane (`cdalar/boxctl-vms`) that drives `onctl -p fc` on its own hosts, a
dashboard (`cdalar/boxctl-web`), and a CLI, `boxctl` (`cdalar/boxctl`) --
including `boxctl claude`, which runs Claude Code itself on a box, with
parallel tasks, session handoff and GitHub/Claude credentials lent from
the laptop.

Two reasons to stop shipping that CLI under its own name:

- **The name is taken, and was first.** `marc-schuetze/boxctl` (MIT,
  created 2025-12-31, ~15 stars) is a Python CLI that also installs a
  `boxctl` binary and also runs coding agents in sandboxes (local Docker).
  Ours dates from Aug-Sep 2026. Both on one `PATH` collide, and search for
  "boxctl" mixes them. The obvious alternatives are no better: npm's
  `onbox` is another microVM-sandbox CLI with an `onbox` binary.
- **onctl is the brand that exists.** Public since 2023, the name people
  find, and boxctl.io is literally onctl's `fc` provider run for you.
  onctl-runners already shows the product-line pattern.

So instead of renaming the CLI, **the hosted service becomes another onctl
provider**, next to hetzner, aws, azure, gcp, ovh, fc, ch and static:

```bash
onctl login                              # once: your boxes token
onctl create -p boxes --type medium      # a box, no cloud account needed
onctl ls -p boxes
onctl ssh -p boxes my-box
onctl claude                             # Claude Code on the project's box
```

There's no `boxctl` binary left to collide with. The service still needs
a name, but only as a provider ID and a website, never as a command on
anyone's `PATH`.

Non-goals: merging the server into onctl (it stays its own service), and
changing the server's API (v1 needs none).

## How the API fits `CloudProviderInterface`

`pkg/cloud.CloudProviderInterface` maps onto the existing boxes API
(`/api/vms*`, bearer personal token) almost method for method:

| onctl | boxes API | Notes |
|---|---|---|
| `Deploy(Vm)` | `POST /api/vms` `{name, image, size, template}` | `Vm.Type` is the size (`small`/`medium`/`large`); `Vm.Image` an image name from the catalog. Blocks until the box is ready (`WaitReady`), like the cloud providers' Deploy. |
| `Destroy(Vm)` | `DELETE /api/vms/{name}` | |
| `Pause(Vm, hot)` | `POST /api/vms/{name}/pause` | A memory snapshot; `hot` is irrelevant. |
| `Resume(Vm)` | `POST /api/vms/{name}/resume` | |
| `List()` | `GET /api/vms` | Running and paused, scoped to the token's owner by the server. |
| `ListPaused()` | -- | Returns empty: `List` already includes paused boxes. |
| `GetByName` | `GET /api/vms` + filter | |
| `CreateSSHKey(file)` | -- | Boxes have no key registry. Returns the public key itself as the "ID"; Deploy authorizes it on the box (below). |
| `SSHInto(...)` | -- | Real ssh, through the tunnel (below). |
| `ImageLister` (optional) | `GET /api/images` | `onctl images -p boxes`. |

Two things in onctl assume more than that interface says, and both need a
hook, because a box has **no address you can reach**: its IP is private to
its host, and the only way in is the server's WebSocket tunnel
(`GET /api/vms/{name}/port/{port}`, the same one `boxctl port-forward`
uses).

### 1. Reaching a box: a dialer

`tools.Remote.connect` does `ssh.Dial("tcp", IP:port)`, and everything
built on it -- `WaitForSSH`, `CopyAndRunRemoteFile` (`-a`), upload and
download (`-u`/`-dl`), `deploy`, `action` -- needs a TCP path to the VM.
`cloud.ProbeSSHReady` TCP-probes the IP too.

Add an optional provider interface:

```go
// Dialer is implemented by providers whose VMs aren't reachable at their
// IP: the connection to a VM port goes through the provider instead.
type Dialer interface {
	DialVM(ctx context.Context, vm Vm, port int) (net.Conn, error)
}
```

`tools.Remote` gains `Dial func(ctx context.Context, port int) (net.Conn,
error)`; when set, `connect` runs `ssh.NewClientConn` over that
connection instead of `ssh.Dial`. `create`, `ssh`, `deploy` and `action`
set it from the provider when it implements `Dialer`. For boxes, `DialVM`
is the tunnel WebSocket wrapped as a `net.Conn` (binary messages are the
byte stream -- what boxctl's `relayPort` already does).

Interactive `onctl ssh` execs the system `ssh`, which can't use a Go
dialer, so it gets a `ProxyCommand` instead:

- A new command, `onctl ssh-proxy <box> [port]`, relays stdin/stdout over
  the tunnel (a port of `boxctl ssh-proxy`; resumes a paused box first).
- `tools.SSHIntoVMRequest` gains `ProxyCommand`; `ProviderBoxes.SSHInto`
  sets it to `<onctl> ssh-proxy -p boxes <box>`.
- It doubles as a way into boxes from anything that speaks ssh:
  `Host *.box / ProxyCommand onctl ssh-proxy -p boxes %n` in
  `~/.ssh/config`, then plain `ssh`, `scp`, `rsync`, git, VS Code
  Remote-SSH.

### 2. Authorizing the key

Every other provider puts the key on the VM at creation (cloud-init,
the provider's key registry, or onctl's own `fc`/`ch` rootfs injection).
The boxes API has no key parameter, so `Deploy` authorizes it right after
the box is up, over the exec endpoint (`POST /api/vms/{name}/exec`, the
one way in that needs no key) -- exactly as `boxctl claude` does today.
A later API change could take the key on create instead; not needed for
v1.

## What onctl gains

### Provider `boxes`

- `internal/providerboxes`: the HTTP client, ported from boxctl's
  `internal/client` (list/create/destroy/pause/resume/exec/images/sizes,
  idle TTL, `DialPort`, `WaitReady`).
- `pkg/cloud/boxes.go`: `ProviderBoxes`, implementing the interface plus
  `Dialer` and `ImageLister`.
- `cmd/root.go`: `case "boxes"` in `initProvider`; `boxes` in the provider
  list.
- Config, in onctl's usual places: `boxes.apiURL` (default the hosted
  service), `boxes.vm.type` (`small`), `boxes.vm.image` (`debian-slim`),
  `boxes.vm.username` (`root`), `boxes.vm.idleTTL` (empty: the server's).
  The token is not config: see `onctl login`.
- `--domain` (a Cloudflare record for the VM's IP) means nothing for a box
  with no public IP: refused with a pointer to `onctl port-forward`.
- `onctl create`'s cloud-init wait is skipped (boxes don't use it), as it
  already is for `fc`/`ch`; `WaitForSSH` runs through the dialer.

### `onctl login` / `onctl logout`

Saves the personal token from the dashboard (hidden prompt or stdin) in
`~/.onctl/boxes.json`, mode 0600 -- the same shape as boxctl's
`~/.boxctl/config.json`, which `onctl login` imports when it finds one, so
nobody pastes a token twice. `ONCTL_BOXES_TOKEN` overrides it, for CI.
Provider-neutral name on purpose: it's the hosted service's login, and the
only provider onctl itself logs in to.

### Optional capabilities, as interfaces

The box-only features become optional interfaces, the way `ImageLister`
and `Restarter` already are, so a command works with whichever providers
can do it and says so cleanly with the rest:

| Interface | Command | Boxes | Others |
|---|---|---|---|
| `Dialer` | `create`/`ssh`/`deploy`/`action` over the provider | the tunnel | -- (direct IP, as today) |
| `PortForwarder` | `onctl port-forward <vm> [LOCAL:]REMOTE...` | the tunnel | later: any provider, via `ssh -L` |
| `SizeLister` | `onctl sizes` (and `--type` completion) | `GET /api/sizes` | later: hetzner/aws instance types |
| `IdleTTLSetter` | `--idle-ttl` on `create` | `PUT /api/vms/{id}/idle-ttl` | -- |

`exec` (boxctl's run-one-command-in-a-fresh-box) and backups
(download/restore) are left for later, deliberately: `onctl import`
already means "adopt an existing host" here, so boxes' import/download
need names of their own -- an open question below.

### `onctl claude`

`boxctl claude` and everything under it -- `--task`, `--own-box`, `ls`,
`fetch`/`push` (with `box/wip`), `--handoff`, `login`/`logout` for the
Claude token, `--github forward|store|off` -- moves to `onctl claude`
largely as it is. It's ~2k lines of Go with tests, already cobra.

v1 runs it on the boxes provider only: it relies on the `claude-agent`
image's tools (gh, tmux, the GitHub credential helper and `gh` wrapper)
and on `ssh-proxy`. But all of it runs over plain ssh, so with `Dialer`
and a setup step that installs those helpers, `onctl claude -p hetzner`
(Claude on your own cloud VM) is a natural follow-up -- with boxes as the
default because they need nothing set up.

The box name prefix (`claude-<project>`) and paths on the box
(`/root/.boxctl/...`, the `box` git remote) don't change: they're inside
the box, not the brand, and keeping them means existing boxes keep
working.

### The Claude Code plugin

Moves from `cdalar/boxctl` into this repo (`claude-plugin/`), renamed
`onctl`: `/onctl:on`, `/onctl:off`, `/onctl:status`, `/onctl:handoff`, its
helper calling `onctl` instead of `boxctl`. `route-bash` keeps commands
starting with `onctl ` local, as it does `boxctl ` today. The marketplace
entry moves with it.

## What happens to the boxctl names

| Thing | Becomes |
|---|---|
| `boxctl` CLI (`cdalar/boxctl`) | A last release that prints "boxctl is now `onctl -p boxes`" on every command, then archived. No real users yet, so no long deprecation. |
| `boxctl-vms` (server) | Unchanged -- its API is the provider's contract. Internal names (`BOXCTL_*`, the repo name) can stay; nobody types them. |
| `boxctl-web` (dashboard) | Unchanged in this plan. Its brand and domain are the one decision left: the proposal is "onctl boxes" at `boxes.onctl.io`, with `boxctl.io` redirecting there. |
| Images (`claude-agent`, ...) | Unchanged. |
| `~/.boxctl/` on users' machines | Read by `onctl login` (the token) and `onctl claude` (the key in `~/.boxctl/claude/`) until a release says otherwise; new state goes to `~/.onctl/`. |

## Rollout

Each step is its own PR here, testable on its own, and none needs the
server to change:

1. **Provider and login.** `providerboxes`, `ProviderBoxes`, `onctl
   login`, `Dialer` + `tools.Remote.Dial`, `ssh-proxy`, `SSHInto` via
   `ProxyCommand`. Done when `create`/`ls`/`ssh`/`pause`/`resume`/
   `destroy`/`images` and `create -a <file>` work with `-p boxes`.
2. **Optional capabilities.** `port-forward`, `sizes`, `--idle-ttl`.
3. **`onctl claude`**, ported with its tests.
4. **The plugin** moves here.
5. **Retire `boxctl`**: deprecation release, archive.

The dashboard rename is independent of all five.

## Testing

As the repo requires: no real cloud in tests. The provider is tested
against an `httptest` server speaking the boxes API (list/create/
pause/...), and the dialer against a WebSocket server that echoes, so
`tools.Remote` runs a real ssh handshake over it with a test key -- the
same test proves `Dialer` for any future provider. `onctl claude`'s
tests come across from boxctl unchanged (they already run its box-side
shell scripts locally). Before each PR: `go test ./...`, `go vet`,
golangci-lint, and one end-to-end pass against boxctl.io with a test
token.

## Open questions

1. **The provider ID.** `boxes` (proposed), `box`, or `onctl`? It's what
   people type after `-p`, and appears in `onctl.yaml`.
2. **`onctl login`** as proposed, or `onctl boxes login`, leaving room for
   other hosted services to log in later?
3. **Backups**: `onctl backup`/`onctl restore` (generic names, boxes-only
   at first), or under a `boxes` command group?
4. **Release coupling.** The hosted product's client now ships with
   onctl's releases; is onctl's cadence fast enough, or should `onctl
   claude` be able to update separately?
5. **Dependencies.** The tunnel needs `gorilla/websocket`, new to this
   module. Acceptable, or use `golang.org/x/net/websocket` /
   `nhooyr.io/websocket`?

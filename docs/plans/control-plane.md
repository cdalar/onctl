# One CLI over a local or remote control plane

Status: proposal, for review. Builds on
[shared-vocabulary.md](shared-vocabulary.md) (Task, Agent, Run,
Workspace, Box).

## Goal

onctl works on its own, with no server: on a laptop or an on-prem
machine with Firecracker it manages boxes, and with a local model it
runs agents too. Pointed at a control plane (boxctl.io, or one you host
with `boxctl-io/boxctl/deploy/onprem`) it does the same through that
server. Agent code is written once.

## What's there today

Checked in `boxctl-io/boxctl` at `2994b92` and `boxctl-io/boxctl-cli` at
`f13d896`.

- **The server drives hosts by running onctl as a command.** A box host
  long-polls `GET /hosts/{name}/jobs/next` and runs what it's given:
  `onctl ls -o json -p fc`, `onctl destroy`, `onctl pause`, `onctl ssh --
  sync` (`server/internal/vm/service.go`). onctl's CLI output is the
  contract between server and host. The server reaches hosts through a
  `dispatcher` interface (`server/internal/vm/dispatcher.go`).
- **The server is a box control plane only.** Its API covers VMs,
  backups, ingress, images, sizes, terminals, exec, usage and MCP. For
  agents it only stores run status, which a box sends its host over vsock
  (`server/internal/runstatus`).
- **The agent layer lives in the boxctl CLI.** Tasks (`backlog.Item`),
  runs and harnesses are there; the backlog is a local file
  (`~/.boxctl/backlog.json`) and `boxctl backlog dispatch` runs its passes
  in the foreground.
- **onctl has a second, smaller copy of the agent layer.** `onctl
  claude` was ported from boxctl and has drifted behind it (no harness,
  goal, issue or backlog).

## Decisions

1. **Open source.** The local control plane, and with it the Task, Run,
   harness and backlog logic, are in onctl, which is open source. What
   only a hosted service needs (accounts, billing, the dashboard,
   admission, usage) stays in the server.
2. **One CLI: onctl.** "boxctl" stays the name of the hosted service and
   the server. The boxctl CLI stays as a thin wrapper: onctl with the
   boxctl.io provider preset, so the hosted service keeps a command of its
   own name.
3. **Internet-free (air-gapped) on-prem installs are supported.** Nothing
   a box or a run needs may be fetched from the internet when the install
   has no route to it: Claude, Kilo, herdr, images and the forge (where
   tasks and pull requests live) must all be reachable inside the
   network.
4. **No backlog store: tasks live on the forge.** The backlog is the
   forge's issues (Azure DevOps: work items), and onctl keeps no task
   database of its own, locally or on the server. See step 4.
5. **Forges: GitHub, GitHub Enterprise Server, Azure DevOps Server
   (TFS) and GitLab (gitlab.com and self-managed).** Azure DevOps Server:
   the current, versionless release (GA December 2025, Modern Lifecycle)
   and 2022 Update 2 (supported until January 2028); 2020 and older are
   out of mainstream support and not targeted. No usage figures by
   version were found, so this follows Microsoft's support lifecycle.
   GitLab: the self-managed versions GitLab itself still supports, over
   its v4 REST API. A forge does two separate jobs: hosting
   code and pull requests, which every run needs, and tracking tasks,
   which only the backlog needs. They're supported separately, so on an
   on-prem forge runs can work before its issues or work items do.
6. **Backlog order comes from labels.** A priority label on each issue
   (Azure DevOps: a tag), the same on every forge; a forge's own board
   or backlog order is not read.
7. **Internal certificate authorities are supported.** An on-prem
   install takes a CA bundle and every box, image, agent, git and the
   runner trust it, so the forge, the control plane, the object store and
   a local model behind an internal CA all work. The install kit
   (`deploy/onprem`) and `onctl` both take the bundle.
8. **Everything onctl and boxctl do is in Go packages; the CLIs only
   parse flags and print.** Boxes, provisioning, connecting, runs,
   harnesses, forges and the control plane are importable packages under
   `pkg/`, which the onctl CLI, the boxctl CLI and the boxctl server all
   call. Package `cmd` keeps flags, prompts, output formats and the tui's
   rendering, nothing else. Package code doesn't print to stdout, prompt,
   or exit the process: it returns values and errors, and takes writers
   for progress and command output. Not in the packages: what only the
   hosted service needs (decision 1: accounts, billing, the dashboard,
   admission, usage). The boxes provider's API client is in: it's how
   either CLI reaches the hosted service.

## Steps

### 1. Shared vocabulary

The renames in [shared-vocabulary.md](shared-vocabulary.md), in onctl and
boxctl-cli. Independent of the rest.

### 2. onctl as a library

- `pkg/box` ([pkg.go.dev](https://pkg.go.dev/github.com/cdalar/onctl/pkg/box)):
  everything onctl does to a box, for any provider (decision 8).
  - Lifecycle: create, list, get, destroy, pause, resume. Done
    (#1348, #1353, #1355, #1356); the CLI and tui go through it.
  - Provider capabilities: images, sizes, restart.
  - Connecting: ssh and exec with the caller's streams and the exit
    code (done, #1355), dialing a box's port, port forwarding, the ssh
    proxy, putting a key on a box.
  - Files: upload and download.
  - Provisioning: waiting for ssh and cloud-init, apply files and
    templates, the domain: all of `onctl create` after the provider has
    the VM, so a caller gets a configured box, not only a created one.
  - Importing hosts (the static provider).
- The host service calls `pkg/box` instead of running `onctl`, behind
  the server's existing `dispatcher` interface. CLI output stops being a
  contract.
- Useful on its own, and the first place duplication goes away.

### 3. A control plane interface in onctl

- One Go interface, `pkg/controlplane`: Boxes, Tasks, Runs. Two
  implementations:
  - **Local**: in the same process, over the step 2 package. No daemon,
    no port.
  - **Remote**: an HTTP client of the server, starting with today's
    `/api/vms`.
- The CLI and tui use only the interface. Standalone means "the local
  implementation"; `onctl login <url>` switches to the remote one.

### 4. The agent layer behind the interface

- **A forge interface**, two parts: code (clone, push, open and watch a
  pull request, its checks) and tasks (list, label, comment on issues or
  work items). GitHub and GitHub Enterprise Server through the same
  client; Azure DevOps Server and GitLab each through their own.
- **Tasks are issues.** What `backlog.json` holds today goes to the
  forge instead:
  - state (queued, active, needs you, done) and priority (decision 6):
    onctl's own labels (Azure DevOps: tags), never the forge's workflow state, so
    an Azure DevOps process (Agile, Scrum, CMMI, Basic) or a custom
    workflow doesn't matter;
  - each run and its outcome (kind, box, done/failed/lost/killed): a
    comment the dispatcher writes on the issue, readable by people and
    parsed back for attempt counts;
  - the run that's going: boxes are already labelled with their issue,
    and the control plane has their run status.
- **One-off runs** (`--task "<text>"`) stay outside the backlog, with no
  issue.
- **The runner opens the pull request, not the agent.** Today the agent
  is told to run `gh pr create`. With several forges the agent only
  commits and pushes; onctl opens the pull request (GitLab: merge
  request) through the forge interface, so images don't need `gh`, `az`
  and `glab`, and the agent's instructions are the same everywhere.
- Agent profiles (harness, model, effort) are files, as harnesses are
  today; the server stores none.
- The agent layer's packages (decision 8): `pkg/run` (workspaces,
  harnesses and agent profiles, runs, sessions, repo and auth copy, wip
  refs, run status) and `pkg/forge`. What `onctl claude` keeps in `cmd`
  today moves there, and the boxctl CLI's copy goes.
- The backlog dispatcher moves out of the boxctl CLI into onctl, behind
  the interface. The run-status channel is reused as it is.
- Still the largest step, but the server gains no task or run
  endpoints.

### 5. One CLI, two-sided tui

- onctl's tui gets boxctl's split: Boxes (boxes, URLs, images, sizes,
  backups) and Board (backlog, runs, PRs, repos), with its home tiles.
- The boxctl CLI becomes the thin wrapper (decision 2). The duplicated
  agent code in both repositories goes.

## Later

- A local daemon, for dispatching the backlog and watching runs after the
  CLI exits.
- gRPC between server and hosts, and between CLI and server, in place of
  long-polled jobs and JSON over HTTP.
- A local model as an agent profile setting (model plus endpoint). Kilo
  first: it speaks to OpenAI-compatible endpoints such as llama.cpp's.
  Claude Code needs an Anthropic-compatible endpoint or a proxy.

## Order

Step 2 can start now. Steps 3 and 4 follow, in that order. Step 5 needs
step 4. Step 1 can land at any time.

## Open questions

None at the moment.

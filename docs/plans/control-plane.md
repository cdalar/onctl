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

## Steps

### 1. Shared vocabulary

The renames in [shared-vocabulary.md](shared-vocabulary.md), in onctl and
boxctl-cli. Independent of the rest.

### 2. onctl as a library

- A stable Go package for what a provider does: create, list, destroy,
  pause, resume, connect. The CLI becomes one caller of it.
- The host service calls that package instead of running `onctl`, behind
  the server's existing `dispatcher` interface. CLI output stops being a
  contract.
- Useful on its own, and the first place duplication goes away. Can start
  now.

### 3. A control plane interface in onctl

- One Go interface: Boxes, Tasks, Runs. Two implementations:
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
  - state (queued, active, needs you, done) and priority: onctl's own
    labels (Azure DevOps: tags), never the forge's workflow state, so
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

1. Ordering: priority labels, or the order of a GitHub Project, an Azure
   DevOps backlog or a GitLab board? Labels work the same on every
   forge.
2. Air-gapped installs usually have an internal certificate authority.
   Boxes, the agent and git must trust it: does the install kit take a
   CA bundle and put it in every image?

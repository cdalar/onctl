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
   the server. The boxctl CLI is retired, or kept as onctl set up for
   boxctl.io; which of the two is decided in step 5.

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

- Tasks, runs and agent profiles (harness, model, effort) are stored by
  the control plane: a file or SQLite locally, new `/api/tasks` and
  `/api/runs` endpoints on the server.
- The backlog dispatcher moves out of the boxctl CLI into onctl, behind
  the interface. The run-status channel is reused as it is.
- The largest step. The server side is new work.

### 5. One CLI, two-sided tui

- onctl's tui gets boxctl's split: Boxes (boxes, URLs, images, sizes,
  backups) and Board (backlog, runs, PRs, repos), with its home tiles.
- The boxctl CLI is retired or becomes onctl preset to boxctl.io. The
  duplicated agent code in both repositories goes.

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

1. Step 4: does the local store start as a JSON file (as the backlog is
   today) or SQLite?
2. Step 5: retire the boxctl CLI, or keep it as a preset onctl?
3. Do internet-free (air-gapped) on-prem installs need to work? Claude,
   Kilo, herdr and images are downloaded today.

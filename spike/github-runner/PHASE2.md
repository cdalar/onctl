# Phase 2 plan: autoscaling controller

## Goal

Replace the manual Phase 1 flow (a human runs `run1.sh`, then triggers a
workflow by hand) with an automated reaction to GitHub's `workflow_job`
webhook: `queued` → provision a JIT runner VM, `completed` → destroy it.
This is ROADMAP.md item #1 (the core gap) and closes item #2 (auto-teardown)
in the same pass.

## Design

A small standalone HTTP service, `spike/github-runner/controller/`
(`go run .`), with no new dependencies (Go stdlib only):

- `POST /webhook` verifies `X-Hub-Signature-256` (HMAC-SHA256), and handles
  `workflow_job` events.
- A job's `labels` must be a superset of `RUNNER_LABELS` (default
  `self-hosted,onctl`) for the controller to act — this ignores GitHub-hosted
  jobs (e.g. `ubuntu-latest`).
- `action: "queued"` → `gh api .../generate-jitconfig` (Phase 1's JIT flow)
  then `onctl create -n gh-runner-<job id> -a github-runner-jit.sh -e JIT_CONFIG=...`,
  run asynchronously (provisioning takes ~1 min, well past GitHub's 10s
  webhook timeout).
- `action: "completed"` → `onctl destroy <runner_name> -f`, where
  `runner_name` comes from the completed event's `workflow_job.runner_name`
  (the name we set via `generate-jitconfig`). The JIT runner has already
  self-deregistered from GitHub (Phase 1 result); this just reclaims the VM
  so it stops billing.
- The VM name `gh-runner-<job id>` is derived from the **queued** job's ID
  at provision time. Teardown does *not* re-derive this from the completed
  job's ID — see Results below for why.

See [controller/README.md](controller/README.md) for env vars and the local
test setup (smee.io relay + webhook registration on `cdalar/onctl-runner-test`).

## Success criteria

- `go test ./...` in `controller/` passes: webhook routing, label filtering,
  and signature verification are exercised against stub `gh`/`onctl`
  binaries without any cloud calls.
- Manual end-to-end: with the controller running and a webhook registered on
  `cdalar/onctl-runner-test`, `gh workflow run onctl-test.yml` results in
  the controller provisioning a VM, the job going green, and the VM being
  destroyed afterwards — no manual `run1.sh`/`onctl destroy` steps.

## Results

- Automated tests: pass (`go test ./...`, 7 tests covering provision,
  teardown via `runner_name`, a completed-without-runner edge case, label
  filtering, bad-signature rejection, and ping).
- Manual end-to-end: run against `cdalar/onctl-runner-test` via smee.io.
  Provisioning worked (`onctl create` ~1m4s, matching Phase 0/1), but
  **teardown failed**: the controller received a `completed` event for an
  older, already-queued job (`gh-runner-81234761816`) — not the job whose
  `queued` event triggered provisioning (`gh-runner-81235253767`) — because
  a runner picks up *any* queued job matching its labels, not necessarily
  the one that caused its creation. Deriving the teardown target from the
  completed job's own ID tried to destroy a VM that was never created under
  that name, leaving `gh-runner-81235253767` running and billing.

  **Fix**: teardown now uses `workflow_job.runner_name` from the completed
  event (the actual VM name) instead of re-deriving a name from the
  completed job's ID. The orphaned VM from this run was destroyed manually.

  **Re-run with the fix: success.** Full automated cycle confirmed —
  `queued` → JIT runner provisioned → job picked up and ran green →
  `completed` → controller destroyed the correct VM. Both ROADMAP items #1
  and #2 are validated end-to-end.

## Out of scope (unchanged ROADMAP items)

GitHub App auth (#3), prebaked images (#4), label-to-template mapping for
multiple concurrent job types (#5), retries/metrics (#6), moving the
bootstrap script to onctl-templates (#7).

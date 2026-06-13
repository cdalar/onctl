# Phase 1 plan: JIT-config runner provisioning

## Goal

Replace Phase 0's registration-token flow with GitHub's just-in-time (JIT)
runner config: a single-use, pre-scoped credential generated *before* the VM
exists (on the machine running `onctl create`), so nothing reusable ever
touches the VM. Same success metrics as Phase 0, plus a check that the runner
process exits on its own (no service install, no token left behind).

## Phase 0 vs Phase 1

| | Phase 0 (registration token) | Phase 1 (JIT config) |
|---|---|---|
| Identity/labels | chosen by `config.sh` on the VM | fixed server-side at `generate-jitconfig` time |
| Credential on VM | registration token (1h, reusable to re-register) | `encoded_jit_config` (one job, one use) |
| VM-side steps | download runner, `config.sh --ephemeral`, `svc.sh install/start` | download runner, `run.sh --jitconfig <blob>` |
| Lifecycle | systemd service, deregisters after 1 job | backgrounded one-shot process, exits after 1 job |

## Steps

1. **Generate the JIT config locally**, before `onctl create`:
   ```bash
   gh api -X POST repos/$GH_REPO/actions/runners/generate-jitconfig \
     -f name=runner-spike-jit -F runner_group_id=1 \
     -f 'labels[]=self-hosted' -f 'labels[]=onctl' \
     -q .encoded_jit_config
   ```
   Requires a token with `administration:write` on the repo (classic PAT with
   `repo` scope, or fine-grained PAT with Administration: Read & write). The
   default `gh auth` token may not have this — verify first.

2. **Pass the blob to the VM** via `-e JIT_CONFIG=<blob>`. Open question:
   the encoded config is a few KB of base64 — confirm onctl's `-e` handles
   that length without truncation before relying on it.

3. **New bootstrap script** (`github-runner-jit.sh`): drop
   `GH_REPO`/`RUNNER_TOKEN`/`config.sh`/`svc.sh` entirely. Steps: install
   packages, optional docker, download runner binary, then
   `./run.sh --jitconfig "$JIT_CONFIG"` in the background (nohup), so
   `onctl create` returns immediately as in Phase 0 — the workflow is
   triggered afterward per the README's manual flow.

4. **Reuse the existing test workflow** (`workflow-onctl-test.yml`) unchanged.

5. **Measure** the same two Phase 0 numbers (provision time, pickup
   latency), plus a third check: after the job, confirm no systemd service
   was installed and the runner process has exited (JIT runners are always
   ephemeral by design).

## Success criteria

- Green workflow run using a JIT-generated runner — no `config.sh` or
  registration token anywhere in the flow.
- Provision time and pickup latency comparable to Phase 0 (~1m / ~few sec).
- Runner process exits after the job with nothing left running.

## Results

Tested via `run1.sh` against `cdalar/onctl-runner-test` on Hetzner:

| | Provision time | Pickup latency | Job result |
|---|---|---|---|
| Phase 1 (JIT config) | 1m1.9s | ~3s | success |

`runner-spike-jit` came online with labels `self-hosted,onctl`, ran the test
workflow (checkout + `docker run hello-world`), and deregistered itself after
the one job (0 runners afterward) — no `config.sh`, no systemd service, no
reusable credential on the VM. All success criteria met.

## Resolved questions

- `runner_group_id=1` worked as-is for this repo-level runner — no error.
- The default `gh auth` session token had sufficient scope for
  `generate-jitconfig`; no separate PAT was needed.
- The base64 `encoded_jit_config` blob passed through onctl's `-e` flag
  without truncation.

## Out of scope

- Warm pools / pre-baked images (Phase 0 README's future-optimization note).
- Org-level runner groups or multi-repo registration.

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
| Lifecycle | systemd service, deregisters after 1 job | one-shot foreground process, exits after 1 job |

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
   `./run.sh --jitconfig "$JIT_CONFIG"` in the foreground.

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

## Open questions / risks

- `runner_group_id`: `1` is the default "Default" group for repo-level
  runners, but confirm the API accepts/needs it for a plain repo (not an
  org) — let the first test call surface this.
- Token scope for `generate-jitconfig` (`administration:write`) may require
  a dedicated PAT distinct from the `gh auth` session token.

## Out of scope

- Warm pools / pre-baked images (Phase 0 README's future-optimization note).
- Org-level runner groups or multi-repo registration.

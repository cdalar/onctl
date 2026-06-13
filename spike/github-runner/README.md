# Phase 0 spike: GitHub Actions runner on an onctl VM

Goal: one green workflow run on an onctl-provisioned Hetzner VM, and a measured
boot-to-job-pickup time.

## Run it

1. Pick a test repo and commit `workflow-onctl-test.yml` there as
   `.github/workflows/onctl-test.yml`.

2. Get a registration token (valid 1 hour):

   ```bash
   export GH_REPO=owner/repo
   TOKEN=$(gh api -X POST "repos/${GH_REPO}/actions/runners/registration-token" -q .token)
   ```

3. Create the runner VM (time the whole thing):

   ```bash
   time onctl create -n runner-spike \
     -a spike/github-runner/github-runner.sh \
     -e GH_REPO=$GH_REPO -e RUNNER_TOKEN=$TOKEN
   ```

4. The runner appears under the repo's Settings → Actions → Runners as
   `runner-spike`, labels `self-hosted,onctl`. Trigger the workflow:

   ```bash
   gh workflow run onctl-test.yml -R $GH_REPO
   gh run watch -R $GH_REPO
   ```

5. Clean up. The runner is **ephemeral** — it deregisters itself after one
   job — so only the VM needs destroying:

   ```bash
   onctl destroy runner-spike
   ```

## What to measure

- **Provision time**: the `time onctl create` output (VM boot + script). The
  script logs `+Ns` offsets per phase — docker install and runner download are
  the two costs a prebaked snapshot would remove.
- **Pickup latency**: from `gh workflow run` to the job's `started_at`
  (`gh run view <id> --json jobs -q '.jobs[0].startedAt'`). With the runner
  already online this should be a few seconds — this is the steady-state
  number a warm pool would deliver.
- Success criterion: green check + both numbers recorded.

## Results

Both phases hit the success criterion (green check + both numbers recorded)
on a Hetzner VM via `onctl create`/`onctl destroy`:

| | Provision time | Pickup latency | Job result |
|---|---|---|---|
| Phase 0 (`run0.sh`, registration token) | 1m5.4s | ~4s | success |
| Phase 1 (`run1.sh`, JIT config) | 1m1.9s | ~3s | success |

Both runs registered a runner (`runner-spike` / `runner-spike-jit`), ran
checkout + `docker run hello-world`, and self-deregistered after the single
job (0 runners left afterward, no leftover systemd service).

## Notes

- Registration tokens are short-lived and single-purpose; nothing reusable is
  left on the VM. The product version (Phase 1) switches to JIT config, which
  is stricter still — see [PHASE1.md](PHASE1.md) for the plan and results.
- `SKIP_DOCKER=1` (via `-e`) skips docker install to isolate its cost when
  comparing against a prebaked image.

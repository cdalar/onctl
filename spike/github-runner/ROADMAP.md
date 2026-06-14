# Roadmap: from spike to product

Phase 0/1 proved the core mechanic: provision a VM via `onctl create`, register
a JIT runner, run one job, self-teardown (~1m provision, ~3s pickup, success).
Everything below is what's still manual today and would need to exist for a
real "on-demand GitHub Actions runners via onctl" product.

## 1. Autoscaling controller (the core gap)

Today: a human runs `run1.sh`, then triggers a workflow by hand. A product
needs the reverse — a `workflow_job: queued` webhook triggers `onctl create`
automatically, matched to the job's requested labels (size/provider/image).
This requires a small always-on service (webhook receiver + provisioning
logic) that doesn't exist yet. This is the actual product; everything else
below is hardening or optimization around it.

**Status: Phase 2 implements this** — see
[PHASE2.md](PHASE2.md) and [controller/](controller/). Label-to-template
mapping beyond a single hardcoded label set/template is still open (#5).

## 2. Auto-teardown / orphan cleanup

Ephemeral runners deregister themselves from GitHub, but the VM keeps
running and billing. Need a `workflow_job: completed` handler (plus a
fallback idle-timeout) that calls `onctl destroy` — otherwise every job
leaks a VM.

**Status: Phase 2 implements the `workflow_job: completed` handler** — see
[PHASE2.md](PHASE2.md). A fallback idle-timeout for jobs that never report
"completed" (e.g. cancelled runs) is still open.

## 3. GitHub App instead of PAT

`generate-jitconfig` currently relies on a personal `gh auth` token
(`administration:write`). A product needs a GitHub App installation per
org/repo — narrower scope, works across customers, no dependency on one
person's credentials.

## 4. Latency: warm pool or prebaked image

~1 min provisioning is mostly docker install + runner download (see the
Phase 0 `+Ns` log offsets). A snapshot/image with both prebaked would cut
this to VM-boot time only — important once jobs queue and wait on cold VMs.

## 5. Concurrency & label-to-template mapping

Currently one VM, one hardcoded template. Real workflows have multiple
concurrent jobs with different label requirements (size, arch, GPU) → needs
a config (e.g. `runners.yaml`) mapping labels to onctl templates/providers.

## 6. Failure handling & observability

No handling today for provisioning failures, runners that never register, or
hung jobs. Needs retries, timeouts, and basic metrics (queue time, success
rate).

## 7. Template location

`github-runner-jit.sh` lives in `spike/`. For real use it belongs in
`onctl-templates` (a JIT variant alongside the registration-token template
already proposed in onctl-templates PR #24).

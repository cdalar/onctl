# Phase 2: autoscaling controller

A small webhook receiver that automates the manual `run1.sh` flow from
Phase 1: on a GitHub `workflow_job: queued` event it generates a JIT runner
config and runs `onctl create`; on `workflow_job: completed` it runs
`onctl destroy` to reclaim the VM. See [../PHASE2.md](../PHASE2.md) for the
plan and results, and [../ROADMAP.md](../ROADMAP.md) items #1 and #2.

## Run it

```bash
export GH_REPO=cdalar/onctl-runner-test
export WEBHOOK_SECRET=$(openssl rand -hex 20)
go run . # listens on :8080 by default
```

### Config (env vars)

| Var | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `GH_REPO` | *(required)* | `owner/repo` |
| `WEBHOOK_SECRET` | *(required)* | shared secret used to verify `X-Hub-Signature-256` |
| `RUNNER_LABELS` | `self-hosted,onctl` | comma list; a job must request all of these labels for the controller to act |
| `BOOTSTRAP_SCRIPT` | `../github-runner-jit.sh` | passed to `onctl create -a` |
| `ONCTL_BIN` | `onctl` | path/name of the onctl binary |
| `GH_BIN` | `gh` | path/name of the gh CLI (needs `administration:write` on `GH_REPO`, same as Phase 1) |

## Local end-to-end test (manual, costs real VM time)

GitHub needs a public URL to deliver webhooks to. For local development use
[smee.io](https://smee.io) as a relay:

```bash
npx smee-client --url https://smee.io/<your-channel> --target http://localhost:8080/webhook
```

Then register a webhook on the test repo:

```bash
gh api repos/$GH_REPO/hooks -X POST \
  -f name=web \
  -f 'config[url]=https://smee.io/<your-channel>' \
  -f 'config[content_type]=json' \
  -f "config[secret]=$WEBHOOK_SECRET" \
  -f 'events[]=workflow_job'
```

With the controller and smee client running, trigger the test workflow:

```bash
gh workflow run onctl-test.yml -R $GH_REPO
gh run watch -R $GH_REPO
```

Expected controller log sequence:

1. `[provision gh-runner-<id> +0s] generating JIT config`
2. `[provision gh-runner-<id> +0s] creating VM`
3. `[provision gh-runner-<id> +Ns] onctl create done` (~1 min, per Phase 1)
4. the workflow run goes green (job picked up in ~seconds once the runner is online)
5. `[teardown gh-runner-<id>] destroying VM` / `done` shortly after the job completes

Clean up the webhook afterwards with `gh api repos/$GH_REPO/hooks/<id> -X DELETE`.

## Automated tests

```bash
go test ./...
```

`main_test.go` points `ONCTL_BIN`/`GH_BIN` at the stub scripts in `testdata/`
so the webhook routing, label filtering, and signature verification can be
checked without any cloud calls.

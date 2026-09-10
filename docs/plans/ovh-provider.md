# Add OVH as a cloud provider (first of OVH / Scaleway / StackIT)

## Context

Following up on the "European sovereign cloud providers" research (`docs/static/notes/`), the user wants onctl to support OVH, Scaleway, and StackIT as deployment targets, alongside the existing aws/azure/gcp/hetzner/fc/ch/static providers — added one at a time. This plan covers **OVH only**. Scaleway and StackIT will each get their own plan/PR later, reusing the same pattern established here.

onctl's provider abstraction (`pkg/cloud.CloudProviderInterface`) is already clean and every existing provider follows the same shape, so this is additive: a new `internal/providerovh` client package, a new `pkg/cloud/ovh.go` implementation, and wiring into `cmd/root.go` / `cmd/create.go` / config, matching Hetzner's implementation as the closest analog (small, non-hyperscaler, simple auth).

## Key OVH API facts (verified against the live OVH API schema, not guessed)

- SDK: `github.com/ovh/go-ovh` — a thin, generic REST client, **not** a typed resource SDK like `hcloud-go`. It exposes `ovh.NewClient(endpoint, appKey, appSecret, consumerKey string) (*Client, error)` and generic `Get/Post/Put/Delete(path string, reqBody, resType interface{}) error` methods. We define our own request/response structs.
- Auth model is fundamentally different from Hetzner's single token: **application key + application secret + consumer key** (env vars `OVH_APPLICATION_KEY`, `OVH_APPLICATION_SECRET`, `OVH_CONSUMER_KEY`, `OVH_ENDPOINT` — default endpoint `ovh-eu`). The consumer key is obtained once, out-of-band, via OVH's token-creation flow (`https://api.ovh.com/createToken/` or the Control Panel) — onctl cannot provision it; this is a documented one-time setup step for the user (like `az login` / `gcloud auth login` today).
- Public Cloud "project" is identified by a `serviceName` (an opaque ID, e.g. from `ovh.CloudProject.list`) — this is a required account-scoped setting, same category as `gcp.project` / `azure.subscriptionId`.
- Confirmed exact JSON fields (from `https://eu.api.ovh.com/1.0/cloud.json`):
  - `POST /cloud/project/{serviceName}/instance` — body: `name` (string, required), `flavorId` (string, required), `imageId` (string), `region` (string, required), `sshKeyId` (string), `monthlyBilling` (bool), `userData` (string — cloud-init goes here, base64 not required, plain text).
  - `GET /cloud/project/{serviceName}/instance/{instanceId}` — response includes `id`, `name`, `status` (enum incl. `ACTIVE`, `BUILD`, `SHUTOFF`, `SHELVED`, `SHELVED_OFFLOADED`, `ERROR`, `DELETING`...), `ipAddresses` (`[]{ip, type("public"/"private"), version}`), `created`, `flavorId`, `imageId`, `region`.
  - `GET /cloud/project/{serviceName}/flavor` — `{id, name, region, vcpus, ram, disk, osType, available}` — **flavor selection is by opaque `id`, not name**; onctl's `--type` (e.g. `d2-4`) must be resolved to an `id` by listing flavors filtered to `region == cfg.Region && name == cfg.VMType`.
  - `GET /cloud/project/{serviceName}/image` — `{id, name, region, type, visibility, user}` — same story: `--image` (e.g. `Ubuntu 22.04`) resolves to `id` by name+region match.
  - `POST /cloud/project/{serviceName}/sshkey` — `{name, publicKey, region(optional)}` → `{id, name, publicKey, regions[]}`. Omitting `region` registers the key for all regions (matches Hetzner's account-wide key model).
  - `DELETE /cloud/project/{serviceName}/instance/{instanceId}`.
  - `POST /cloud/project/{serviceName}/instance/{instanceId}/shelve` and `.../unshelve` — **this is OVH's built-in stop-billing/restore**, directly analogous to what Hetzner's `Pause`/`Resume` fake via manual snapshot+delete+recreate. Use these instead of reinventing snapshot logic.
- **No tags/labels on instances** in either the legacy or v2 (`CreateInput`) instance schema, and no `/tag` endpoints under `/cloud/project/{serviceName}`. Unlike Hetzner/AWS/GCP/Azure (which scope `List`/`Destroy` to `Owner=onctl` labels), OVH has no server-side way to mark "onctl-owned" instances.
  - **Decision (confirmed with user): the whole configured OVH project is treated as onctl's.** `List()` returns every instance in `serviceName`, no filtering. Document this prominently (onctl.yaml comment + README) so nobody points onctl at a project with unrelated VMs.

## New files

### `internal/providerovh/common.go`
Mirrors `internal/providerhtz/common.go`'s shape (`GetClient()` returning the SDK client, hard-exit on missing creds — this codebase's established style for simple-token providers) but sourcing 4 env vars instead of 1:
```go
func GetClient() *ovh.Client {
    endpoint := os.Getenv("OVH_ENDPOINT")
    if endpoint == "" {
        endpoint = "ovh-eu"
    }
    appKey := os.Getenv("OVH_APPLICATION_KEY")
    appSecret := os.Getenv("OVH_APPLICATION_SECRET")
    consumerKey := os.Getenv("OVH_CONSUMER_KEY")
    if appKey == "" || appSecret == "" || consumerKey == "" {
        log.Println("OVH_APPLICATION_KEY, OVH_APPLICATION_SECRET and OVH_CONSUMER_KEY must all be set")
        os.Exit(1)
    }
    client, err := ovh.NewClient(endpoint, appKey, appSecret, consumerKey)
    if err != nil {
        log.Fatalln(err)
    }
    return client
}
```

### `pkg/cloud/ovh.go`
```go
type OvhConfig struct {
    ServiceName   string // required: OVH Public Cloud project id
    Region        string // e.g. "GRA11"
    VMType        string // flavor name, e.g. "d2-4"
    Image         string // image name, e.g. "Ubuntu 22.04"
    Username      string
    SSHPrivateKey string
}

// ovhAPIClient is the subset of *ovh.Client's generic REST methods we use.
// Defined as an interface (rather than depending on *ovh.Client directly) so
// tests can inject a fake — go-ovh has no documented httptest-friendly
// endpoint override, unlike hcloud-go's WithEndpoint used in
// hetzner_images_test.go.
type ovhAPIClient interface {
    Get(path string, resType interface{}) error
    Post(path string, reqBody, resType interface{}) error
    Delete(path string, resType interface{}) error
}

type ProviderOvh struct {
    Client ovhAPIClient
    Config OvhConfig
}
```
Implements `CloudProviderInterface`:
- **Deploy**: resolve `flavorId` (GET `/flavor`, filter region+name), resolve `imageId` (GET `/image`, filter region+name) unless `server.Image`/`p.Config.Image` overrides, POST `/instance` with `userData` = cloud-init file contents (reuse `tools.FileToBase64`? — check: OVH's `userData` field is plain string, not base64 like Hetzner's; use plain file contents, note this difference in a comment), then poll GET `/instance/{id}` until `status == "ACTIVE"` (bounded by the existing `--cloud-init-timeout`-style pattern / a sane fixed poll loop with timeout+backoff, logged at `[DEBUG]`) to get `ipAddresses` before returning the mapped `Vm`.
- **Destroy**: resolve ID via `GetByName` if `server.ID` empty (matches Hetzner's pattern), `DELETE /instance/{id}`.
- **Pause(server, hot)**: `POST /instance/{id}/shelve`. OVH's shelve already handles the stop-then-offload sequencing server-side, so `hot` has no effect here (comment explaining why, in this codebase's established style of explaining cross-provider asymmetries — see `cloud.go`'s `SSHReady` comment).
- **Resume**: `POST /instance/{id}/unshelve`, then poll until `ACTIVE`.
- **List**: `GET /instance` (all instances in the project — see scoping decision above), map every entry.
- **ListPaused**: filter `List()`'s raw instance data (or a second `GET /instance` pass) to `status` in `{SHELVED, SHELVED_OFFLOADED}` — no separate snapshot bookkeeping needed, unlike Hetzner.
- **CreateSSHKey**: GET `/sshkey`, match existing by `publicKey` content (avoids relying on a specific OVH conflict-error code we haven't confirmed) — return existing `id` if found, else POST to create, name `"onctl-" + md5(pubkey)[:8]` (same convention as Hetzner).
- **GetByName**: GET `/instance`, filter by `name` client-side (OVH's list endpoint takes no name-filter query param per the schema).
- **SSHInto**: reuse `tools.SSHIntoVM` exactly like Hetzner's, resolving the instance's public IP via `GetByName`.
- Mapping helper `mapOvhInstance(...)` → `Vm{Provider: "ovh", ...}`. No live pricing data available inline like Hetzner's flavor pricings in this legacy API, so `Cost` is left zero-valued (matches `ch`/`fc`'s local-VM zero-cost convention) — do **not** fabricate a price.

## Wiring changes

- `cmd/root.go`:
  - `cloudProviderList` (line 70): append `"ovh"`.
  - `initProvider` switch (line 193): add `case "ovh"` constructing `&cloud.ProviderOvh{Client: providerovh.GetClient(), Config: cloud.OvhConfig{...viper reads...}}`.
  - Add a required-setting resolver analogous to `resolveGCPProject`/`resolveAzureIdentifiers` — `resolveOvhServiceName` — since there's no local CLI to shell out to for a default, just validate it's set and return an actionable error (`ovh.serviceName is required: set --service-name, or edit .onctl/onctl.yaml`) if it's still the placeholder.
  - Register a persistent `--service-name` flag (next to `--project`/`--subscription-id`) bound to `ovh.serviceName`.
- `internal/tools/common.go` `WhichCloudProvider()`: add `if os.Getenv("OVH_APPLICATION_KEY") != "" { return "ovh" }`.
- `cmd/create.go`: bind `--type`→`ovh.vm.type`, `--location`→`ovh.location`, `--username`→`ovh.vm.username`, `--image`→`ovh.vm.image` (mirror hetzner's 4 bindings at lines 110-116); extend the image-flag allowlist at line 211 to include `"ovh"`.
- `cmd/list.go`: extend `isPausedStatus` (lines 141-145) to also match OVH's `"SHELVED"`/`"SHELVED_OFFLOADED"` status strings.
- `cmd/images.go`: leave hetzner-only for now (OVH's image list needs a region to be useful; not in scope for this first pass — call out as a possible follow-up, not silently dropped).
- `internal/files/init/onctl.yaml`: new block, mirroring hetzner's shape plus the required placeholder convention gcp/azure use:
  ```yaml
  # ---- OVHcloud ----
  # Requires OVH_APPLICATION_KEY, OVH_APPLICATION_SECRET, OVH_CONSUMER_KEY env vars
  # (see https://api.ovh.com/createToken/ for one-time setup).
  # NOTE: onctl treats every instance in this OVH Public Cloud project as its
  # own (OVH has no server-side tagging) -- don't point it at a project that
  # has unrelated VMs.
  ovh:
    serviceName: <service-name>      # REQUIRED: your OVH Public Cloud project ID
    location: GRA11                  # region
    vm:
      type: d2-4                     # flavor name
      username: ubuntu                # ssh user (OVH's default cloud images use "ubuntu"/"debian" etc.)
      image: Ubuntu 22.04             # OS image name
  ```
- `go.mod`/`go.sum`: `go get github.com/ovh/go-ovh`.

## Docs

- `README.md`: update the provider list (line 20) and the `ONCTL_CLOUD=` examples to mention `ovh`.
- `docs/docs/getting-started.md`: add `ovh` to the supported `ONCTL_CLOUD` values list, and document the OVH-specific one-time consumer-key setup step.
- `docs/docs/index.md`: update the provider list and the `--provider` flag help text.

## Tests

- `pkg/cloud/pause_resume_test.go`: add `ProviderOvh{}` to both the compile-time `var (...)` block and the `providers` map.
- New `pkg/cloud/ovh_test.go`: a hand-rolled fake implementing `ovhAPIClient` (records calls, returns canned JSON per path — same spirit as `hetzner_images_test.go`'s `httptest`-backed approach, just at the interface level since go-ovh doesn't offer a clean endpoint override). Cover:
  - Deploy: flavor/image name→id resolution, instance creation payload shape, polling-until-ACTIVE, IP mapping.
  - Pause/Resume: correct endpoints hit (`shelve`/`unshelve`), status mapping.
  - CreateSSHKey: idempotent match-by-public-key-content path.
  - GetByName / Destroy: not-found handling.
- Table-driven where it mirrors existing style (see `hetzner_images_test.go`'s `hcloudImage` table test for the pattern).

## Verification

1. `go build ./...` and `go vet ./...` after adding the new package/files.
2. `go test ./pkg/cloud/... ./internal/providerovh/...` — new fake-client tests plus the existing `pause_resume_test.go` contract test must pass.
3. Manual smoke test against a real OVH Public Cloud account (needs the user's own OVH credentials — flag this as a manual step I can't complete myself): `onctl init`, fill in `ovh.serviceName`, `export OVH_APPLICATION_KEY=... OVH_APPLICATION_SECRET=... OVH_CONSUMER_KEY=...`, then `onctl create -p ovh`, `onctl ls -p ovh`, `onctl pause <name> -p ovh`, `onctl resume <name> -p ovh`, `onctl destroy <name> -p ovh`. Several field values (exact `monthlyBilling` default behavior, real poll timing for `BUILD`→`ACTIVE`, exact OVH conflict error code for duplicate SSH keys/instance names) are documented from the API schema but not yet exercised against a live account — call this out explicitly rather than claiming full verification.
4. Follow the existing branch-per-feature / PR workflow (see prior sessions in this repo): new branch off `main`, PR with the standard summary/test-plan format.

## Explicitly out of scope here (follow-up work)

- Scaleway and StackIT providers — separate plans/PRs, once OVH's pattern is validated.
- `onctl images` support for OVH (needs a region-scoped image list UX decision).
- Any live pricing/cost display for OVH instances (the legacy instance API doesn't expose it inline the way Hetzner's does).

# Business Plan: BYO-Cloud GitHub Actions Runners (built on onctl)

*Research date: June 12, 2026*

## The idea

Sell managed GitHub Actions runners that run **in the customer's own cloud account**
(Hetzner first, then GCP/Azure/AWS), provisioned and recycled by onctl's existing
VM + Firecracker engine. Flat annual license — "RunsOn, but multi-cloud."

**Pitch:** *GitHub Actions runners in your Hetzner account. 10x cheaper. Flat fee. One command.*

## Why this is worth the time

1. **Buying moment, right now.** GitHub announced a $0.002/min fee on self-hosted
   runners (March 2026), then postponed it under community backlash, while cutting
   hosted-runner prices 15–39%. Every team is re-modeling CI spend this year.
2. **Proof people pay.** Blacksmith (YC W24): $17.6M raised, $1M ARR tripling in
   4 months, 1,000+ orgs, 20M+ jobs/month. WarpBuild, Depot, Namespace, Tenki all
   compete and survive.
3. **Existence proof at solo scale.** RunsOn is a flat-license, runs-in-your-AWS-account
   product run by a single founder.
4. **Open wedge.** RunsOn is AWS-only. Nobody serves Hetzner/GCP/Azure with this
   model. Hetzner gives the most dramatic cost headline of any cloud.
5. **~80% code reuse.** Ephemeral VM create/destroy, SSH bootstrap, multi-cloud
   abstraction, and Firecracker fast-boot isolation is literally the runner-provider
   stack onctl already has.
6. **Solo-operable economics.** License model = no 24/7 hosted fleet, no compute
   resale margin, no abuse/crypto-mining problem.

## Market report

### Market size and structure

- GitHub Actions powers ~85% of CI/CD pipelines on GitHub, ~92M workflow builds/month.
- Global CI/CD tools market: ~$13.2B (2026) → $22.9B (2033), 8.2% CAGR.
- AI tailwind: Blacksmith's Series A thesis is that AI coding agents multiply CI volume.

### Two business models

| Model | Players | Economics |
|---|---|---|
| **A. Managed hosted runners** | Blacksmith, WarpBuild, Depot, Namespace, Tenki | Per-minute billing, priced at ~half of GitHub; margins compressing toward $0.003/min floor |
| **B. BYO-cloud license** | RunsOn (AWS-only), WarpBuild BYOC tier | Flat license, customer pays raw cloud/spot cost, 7–15x savings claims |

### Pricing benchmark (Linux runners, 2026)

| Provider | 2-vCPU | 4-vCPU | Model |
|---|---|---|---|
| GitHub-hosted | $0.008/min | $0.016/min | hosted |
| WarpBuild | $0.004 | $0.008 | hosted + BYOC |
| Blacksmith | $0.004 | $0.008 | hosted |
| Namespace | $0.003 | $0.008 | hosted, prepaid |
| Tenki | $0.003 | $0.006 | hosted |
| RunsOn | raw AWS/spot | raw cost | flat annual license |

### Funding / traction

- **Blacksmith**: $10M Series A (Google Ventures, closed in 14 days), $17.6M total.
  $1M ARR, revenue 3x in 4 months. Customers: Supabase, Clerk, Ashby, Mintlify.
- **Daytona/E2B** (adjacent AI-sandbox space): $24M / $35M raised — relevant because
  the same Firecracker engine serves both markets.
- **RunsOn**: solo founder, open GitHub repo + CloudFormation/Terraform one-stack
  install, flat license, free for non-commercial.

### Risks

- **Platform risk (the big one):** GitHub's self-hosted fee is postponed, not
  cancelled. At $0.002/min it shaves but doesn't kill BYO-cloud savings (still
  60–90% on spot). If GitHub escalates against third-party runners, the whole
  category inherits the risk. **Hedge:** same engine works for GitLab CI and
  Buildkite agents; no incumbent does multi-forge well.
- **Hosted-segment knife fight:** avoid Model A entirely; don't resell compute.
- **Hetzner capacity/abuse policies** for CI workloads need early validation.

## Validation plan (cheap, falsifiable)

1. Landing page: "GitHub Actions runners in your Hetzner account, 10x cheaper, flat $X/yr."
2. Post where the GitHub self-hosted-fee outrage is live (community discussion
   #182089, HN, r/devops).
3. Talk to ~10 teams currently paying >$500/mo for Actions minutes.
4. Kill criterion: if the pricing chaos can't produce 10 conversations, stop.
5. If validated: MVP = onctl engine + GitHub runner registration + job-driven
   autoscaling + Terraform/one-command install. Keep onctl OSS as the funnel.

## Secondary option (shares the codebase)

Self-hostable AI agent sandbox layer — "E2B you run in your own VPC" — for
enterprises that can't send agent-generated code to a third-party cloud. Hosted
players are heavily funded (E2B $35M, Daytona $24M) and commoditizing; the
self-hosted enterprise gap is open. Higher bar (SDK, snapshot/restore, sub-second
cold starts) but the Firecracker provider is the seed of it.

## Sources

- Tenki: <https://tenki.cloud/blog/github-actions-runner-showdown-2026>
- Blacksmith raise: <https://theaiinsider.tech/2025/09/26/blacksmith-raises-10m-to-unblock-ai-development-with-fast-ci-for-github-actions/>
- Blacksmith YC profile: <https://www.ycombinator.com/companies/blacksmith>
- RunsOn pricing: <https://runs-on.com/pricing/>
- RunsOn fee analysis: <https://runs-on.com/blog/github-self-hosted-runner-fee-2026/>
- RunsOn repo: <https://github.com/runs-on/runs-on>
- WarpBuild pricing: <https://www.warpbuild.com/pricing>
- GitHub pricing changes: <https://github.com/resources/insights/2026-pricing-changes-for-github-actions>
- Fee postponement: <https://www.devclass.com/development/2025/12/17/github-to-charge-for-self-hosted-runners-from-march-2026/1734518>
- Community backlash: <https://github.com/orgs/community/discussions/182089>
- CI/CD market sizing: <https://www.persistencemarketresearch.com/market-research/continuous-integration-and-delivery-ci-cd-tools-market.asp>
- Actions market share: <https://tech-insider.org/jenkins-vs-github-actions-2026/>
- Northflank overview: <https://northflank.com/blog/github-pricing-change-self-hosted-alternatives-github-actions>
- AI sandbox comparison: <https://www.startuphub.ai/ai-news/artificial-intelligence/2026/daytona-vs-e2b-vs-modal-vs-vercel-sandbox-2026>

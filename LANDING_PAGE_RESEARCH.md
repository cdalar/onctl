# onctl.sh Landing Page — Comparative Research

Research into landing-page design patterns for comparable single-binary/OSS CLI tool
websites, done to inform the design of a real landing page for onctl.sh (currently
just the Docusaurus docs homepage, no dedicated landing page).

12 live CLI/dev-tool homepages, fetched directly (September 2026 snapshots). Two
redirected to new domains during research: `charm.sh` → `charm.land`,
`multipass.run` → `canonical.com/multipass`.

## Summary Table

| Site | Style category | Terminal demo? | Install cmd in hero? | Dark/light default | Notable feature |
|---|---|---|---|---|---|
| **bun.sh** | Highly produced marketing site | Yes — animated 5-step replay demo | Yes, with OS toggle + copy | Dark | Full benchmark suite vs Node/Deno/npm/yarn/pnpm with methodology links |
| **rustup.rs** | Minimal indie utility | No | Yes, plain text, "Copied!" indicator | Unstyled/minimal | Platform auto-detection swaps the install command shown |
| **astral.sh** | Polished but restrained (corp OSS) | No | No — install is in per-product page, not root hero | Dark | Testimonial quotes emphasizing raw speed ("nearly 1000x faster") |
| **starship.rs** | Clean minimal docs-site | No | No — install pushed below fold | Dark | 10+ shell tabs (Nushell, Elvish, Xonsh...) for install instructions |
| **charm.land** (was charm.sh) | Distinctive indie/whimsical branding | No (surprisingly, given VHS) | No | Retro-terminal themed, mascots | Product grid of 8+ named libraries with individual mascots ("Bubbles," "Glamour," "Lip Gloss") |
| **atuin.sh** | Produced, SaaS-adjacent OSS | No (mock terminal UI instead) | Yes, with copy button | Green-accented, modern | Adoption stats bar (30K★, 300+ contributors, 600M+ synced commands) + 17 company logos |
| **zellij.dev** | Clean minimal docs-site | No | Yes, 3 shell variants, copy buttons | Light-leaning | Sponsor link in primary nav; hosted-service upsell in footer |
| **multipass.run → canonical.com** | Corporate product page | No | No — "Install now" CTA only, no command shown | Light, Ubuntu orange | Fully absorbed into Canonical's corporate site chrome (Products/Solutions/Partners nav) |
| **k9scli.io** | Bare-bones indie/functional | Asciinema link (not embedded) | No | Light, plain | Single-link nav; personal maintainer contact info in community section |
| **cli.github.com** | Official-brand polish, restrained | No | Yes (`brew install gh`), copy + highlighting | GitHub light/dark toggle | Command showcase of real workflows (`gh pr checkout`, `gh copilot`, etc.) as the "demo" instead of a GIF |
| **brew.sh** | Deliberately minimal/indie | GIF of terminal install running | Yes, plain block, no copy button visible | Light, no dark mode | Explicit "Donate" section (GH Sponsors/Open Collective/Patreon); credits named individual maintainers |
| **httpie.io** | Full SaaS/commercial marketing site | No | No — install buried below fold | Light w/ dark toggle | Multi-tier product nav (Desktop/Terminal/AI/Jobs), GitHub star count as trust badge, newsletter signup |

## The 3–4 Most Common Patterns

1. **The install command is the real hero, but its prominence varies by ambition.**
   Every tool exposes a single copy-pasteable curl/brew command, but only the more
   product-minded sites (bun, atuin, zellij, GitHub CLI, Homebrew) put it literally
   above the fold with a copy button. The purer "reference implementations" (rustup,
   starship, astral, multipass, k9s, httpie) push it down a scroll or bury it behind
   a CTA click.
2. **Nobody uses a live embedded terminal/asciinema player in the actual hero —
   animated *replays* substitute for it.** Even the most produced site (bun.sh) uses
   a scripted step-by-step replay below the fold, not an autoplaying terminal in the
   hero itself. Homebrew uses a static GIF. k9s links out to asciinema rather than
   embedding. This suggests real terminal demos are more effort/fragility than most
   teams find worth it in the hero specifically — they're used as supporting evidence
   further down.
3. **Dark-mode-by-default is the majority default among the technical/indie set**
   (bun, astral, starship, atuin, zellij-leaning) while the more corporate/mainstream
   -facing ones (GitHub CLI, Homebrew, multipass, httpie) stay light or
   light-with-toggle. Monospace is used consistently for code, sans-serif for prose —
   no exceptions found.
4. **Nearly every site ends with a lightweight community/trust block** — GitHub
   stars, contributor counts, Discord/Slack links, or company logos — rather than a
   heavy sales-y testimonial carousel. Atuin and httpie are the two outliers going
   further into stats/logos/testimonials territory.

## The Clearest Split

**Pole A — "docs-site-that-happens-to-have-a-homepage"** (rustup, starship, k9s,
brew.sh, and to a real extent multipass since it's absorbed into Canonical's generic
template): sparse hero, install command doubles as most of the content, little to no
below-the-fold marketing apparatus, functional typography, often no dark mode. These
read as maintained-by-a-small-team-who-would-rather-be-writing-code sites.

**Pole B — "produced marketing site"** (bun.sh, httpie.io, and increasingly
atuin.sh): full benchmark sections, testimonial quotes, company-logo walls,
multi-product navigation, animated demos, professional copywriting ("FLOW THROUGH
APIs"). These read as venture-backed or commercially-motivated products even when
open-source at the core.

**charm.land** is its own third axis — not more "produced" in the SaaS sense, but
distinctive through *personality/branding* (mascots, playful naming, retro terminal
aesthetic) rather than production value or marketing apparatus.

**Where onctl should land:** onctl is a small OSS team, docs-first,
technical-audience, infra-adjacent tool (VM lifecycle management) — this is much
closer to the **rustup/starship/zellij/k9s cluster** than to bun/httpie. Multipass is
the closest *category* match (local VM CLI) but its current incarnation is a
Canonical corporate page, which is not a good template for a small team — it would
read as aspirational/incongruous. The better target is **"zellij/starship-tier
polish without httpie-tier marketing apparatus"**: clean, dark-default,
monospace-forward, install-command-prominent, one clear feature grid, no fake logo
walls or testimonial carousels the team doesn't actually have. This is honest to
onctl's actual size while still looking modern and cared-for (not as bare as
rustup.rs or k9scli.io, which read a little neglected).

## Concrete Recommendations for onctl.sh

1. **Put the install command in the hero with a copy button**, styled like
   Homebrew/Bun/Atuin — but add the OS/shell awareness that rustup and zellij do.
   Since onctl targets multiple clouds *and* local hypervisors, show
   `curl -fsSL https://onctl.sh/get.sh | bash` immediately below a one-line headline,
   with a small monospace block and copy icon. Skip syntax highlighting theatrics — a
   clean dark code block with a copy button (as zellij and GitHub CLI do) is
   sufficient and matches the target tier.

2. **Skip a hero terminal GIF/asciinema embed** — none of the comps actually do this
   in the hero, and it's not worth the production cost for a small team. Instead, do
   what GitHub CLI does: show 3–4 real command examples below the fold
   (`onctl create -p aws`, `onctl create -p hetzner`, `onctl create -p fc` for local
   Firecracker, `onctl ls`) as static styled terminal-output blocks. This
   communicates the multi-cloud value prop concretely without needing video/animation
   tooling.

3. **Lead the feature section with a "one CLI, N backends" grid** (AWS / Azure / GCP
   / Hetzner / Firecracker-local) — this is onctl's actual differentiator vs.
   single-cloud or single-hypervisor tools, and no comp here has quite this
   "provider matrix" story to tell except multipass (which only does local) and
   bun/astral (whose grids are "tools," not "backends"). A visual grid of cloud
   logos + a local/microVM icon, each linking to its docs page, would immediately
   clarify scope that a text paragraph won't.

4. **Include a small, honest trust bar, but don't fake scale.** Show real GitHub
   stars (via shields.io or the GitHub API badge, as GitHub CLI and httpie do) and a
   link to Discussions/Issues — skip logo walls or testimonials until onctl actually
   has recognizable adopters; atuin's and httpie's logo/testimonial sections work
   because they have genuine numbers to back it, and a thin imitation would undercut
   credibility for a docs-first technical audience that will notice.

5. **Structure top nav minimally: Docs | GitHub | (maybe Blog/Changelog) | Get
   Started/Install button** — mirroring zellij/starship rather than httpie's
   multi-tier product nav or Canonical's corporate mega-menu. Footer should stay
   equally lean: links to Docs, GitHub, install script source, license, and one or
   two community channels (Discord/Discussions) — Homebrew's footer (crediting
   maintainers, linking donation/sponsor options) is a good honest-small-team model
   to follow if onctl wants a sponsor/donate link.

## Sources

- [bun.sh](https://bun.sh)
- [rustup.rs](https://rustup.rs)
- [astral.sh](https://astral.sh)
- [starship.rs](https://starship.rs)
- [charm.land](http://charm.land/) (redirected from charm.sh)
- [atuin.sh](https://atuin.sh)
- [zellij.dev](https://zellij.dev)
- [canonical.com/multipass](https://canonical.com/multipass) (redirected from multipass.run)
- [k9scli.io](https://k9scli.io)
- [cli.github.com](https://cli.github.com)
- [brew.sh](https://brew.sh)
- [httpie.io](https://httpie.io)

---

# Tech Stack Findings

The same 12 sites, this time fingerprinted for actual framework/hosting/tooling
(response headers, HTML markers, and public site-source repos where findable) rather
than design patterns.

| Site | Framework/SSG | CSS approach | Hosting/CDN | Site source repo | Notes |
|---|---|---|---|---|---|
| **bun.sh** | No mainstream framework fingerprint; uses Pagefind for search | Unconfirmed | Vercel, Cloudflare in front | Closed-source | Canonical URL is actually `bun.com` |
| **rustup.rs** | None — hand-rolled static HTML | Plain custom CSS | AWS S3 + CloudFront | Not confirmed | Simplest stack of the whole set |
| **astral.sh** | Next.js (App Router) — confirmed via RSC headers | Next.js defaults | Vercel, Cloudflare in front | Closed-source | |
| **starship.rs** | VitePress v1.6.4 — confirmed via generator tag | VitePress Vue-scoped theme | Netlify | Likely in `starship/starship` monorepo | |
| **charm.land** | Custom SPA, hashed bundles, no framework fingerprint | Custom | AWS S3 + CloudFront | Closed-source | Video-background hero, 3D `<model-viewer>` |
| **atuin.sh** | None — hand-rolled HTML/CSS/JS | Hand-written, Google Fonts | Cloudflare | No public site repo found | Polish is typography/copy, not framework |
| **zellij.dev** | Hugo — confirmed via generator tag + public repo | Hugo theme | GitHub Pages | [zellij-org/zellij-org.github.io](https://github.com/zellij-org/zellij-org.github.io) | Fully open-source |
| **canonical.com/multipass** | Django CMS | Canonical's Vanilla Framework | Self-hosted nginx | Internal, N/A | Absorbed into corporate CMS |
| **k9scli.io** | Jekyll (GitHub Pages default) | Jekyll "minima" theme | GitHub Pages | Likely in `derailed/k9s` | |
| **cli.github.com** | Jekyll v4.4.1 — confirmed | Jekyll theme | GitHub Pages | Not public | |
| **brew.sh** | Jekyll v4.4.1 — confirmed + public repo | Jekyll | GitHub Pages | [Homebrew/brew.sh](https://github.com/Homebrew/brew.sh) | Fully open-source |
| **httpie.io** | Next.js — confirmed | Next.js defaults | Vercel, Cloudflare in front | Closed-source (commercial, separate from OSS `httpie/cli`) | |

## Synthesis

Two clear clusters, matching the design-pattern split above:

- **SSG + GitHub Pages** (Jekyll ×3: k9scli.io, cli.github.com, brew.sh; Hugo ×1:
  zellij.dev) — zero-config, free hosting, matches the "docs-site-that-happens-to-
  have-a-homepage" pole exactly. Dominant pattern among the small-team/indie-OSS
  cluster.
- **Next.js + Vercel** (astral.sh, httpie.io, likely bun.sh) — matches the "produced
  marketing site" pole; unsurprisingly none of these have public site source.

The pure hand-rolled tier (rustup.rs on raw S3+CloudFront, atuin.sh with no
framework at all) shows that visual polish (atuin.sh looks quite finished) doesn't
require a framework — it's achievable with plain HTML/CSS plus one small custom JS
widget for a demo.

## Recommendation for onctl.sh

`onctl.sh` **is already a Docusaurus site** (a React-based SSG, same category as
Hugo/Jekyll/VitePress) deployed via GitHub Pages — i.e., it already sits in exactly
the stack cluster this research says fits onctl's positioning. Building a separate
static site (a second Vite/Next/Astro build deployed elsewhere) would mean
maintaining two toolchains and two deploy pipelines for one small domain, and would
contradict the "small team, docs-first" identity the design research recommends
leaning into.

**Concretely: build the landing page as a Docusaurus custom page —
`docs/src/pages/index.tsx`** (Docusaurus's built-in mechanism for exactly this: a
non-docs homepage sharing the same build, theme, deploy workflow, and
`docusaurus-lunr-search` setup already in place). This is a same-repo, same-CI,
zero-new-infrastructure change — `docu-deploy.yml` already builds and publishes this
site on every push to `main`, so a new `index.tsx` just ships through the existing
pipeline. No new stack decision needed at all.

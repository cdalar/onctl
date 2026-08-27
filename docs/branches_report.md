# Branch Report for origin

## Base
- `origin/main`: The primary stable branch containing the production-ready codebase.

## AI-Assisted
- `origin/ai/fix-cursor-visibility`: Fixes issues related to cursor visibility using AI assistance.
- `origin/ai/update-proxmox-715`: Updates Proxmox integration to version 7.1.5 via AI.
- `origin/claude/practical-hypatia-*`: Multiple branches used for specialized development sessions with Claude.

## Features
- `origin/aws.region`: Implements or configures support for AWS regions.
- `origin/azure-output`: Enhances the output format for Azure-related commands.
- `origin/feat/azure-zero-yaml-flags`: Enables Azure configuration without requiring YAML flags.
- `origin/feat/gcp-zero-yaml-flags`: Enables GCP configuration without requiring YAML flags.
- `origin/feat/ls-multi-provider`: Adds support for listing resources across multiple providers in the `ls` command.
- `origin/login`: Implements or refactors authentication and login workflows.
- `origin/network`: Implements new network-related functionality or configurations.
- `origin/ssh`: Enhances or refactors SSH connectivity features.
- `origin/ssh-dotenv`: Adds support for using `.env` files for SSH configurations.

## Cloud Providers
- `origin/oraclecloud`: Implements support for Oracle Cloud infrastructure.
- `origin/proxmox`: Implements support for the Proxmox virtualization platform.
- `origin/vsphere`: Implements support for VMware vSphere virtualization.

## Infrastructure & CI/CD
- `origin/design/pipelines`: Contains design proposals or implementations for new CI/CD pipelines.
- `origin/pipeline`: Manages changes related to the project's automation pipelines.
- `origin/sign-release`: Handles the process of signing software releases.
- `origin/self-hosted`: Configures or implements support for self-hosted environments.
- `origin/templates`: Manages deployment or configuration templates.
- `origin/vnet-fix`: Addresses issues specifically within Virtual Network configurations.

## Refactoring & Migrations
- `origin/worktree-fix-fc-defaults`: Fixes Firecracker default settings using a dedicated worktree.
- `origin/worktree-hetzner-viper-decouple`: Decouples Hetzner integration from the Viper library.
- `origin/worktree-install-dev-script`: Updates or creates scripts for developer environment setup.
- `origin/worktree-phase1-hetzner-decouple`: Executes the first phase of decoupling Hetzner components.
- `origin/worktree-pkg-cloud-rename`: Performs renaming of cloud-related packages or modules.

## Maintenance & Security
- `origin/clean`: Performs general codebase cleanup and organization.
- `origin/dependabot/go_modules/...`: Automated branches for updating Go module dependencies.
- `origin/snyk`: Addresses security vulnerabilities identified by Snyk.

## Benchmarks & Tools
- `origin/benchmark/onctl-vs-github-runners`: Compares the performance of `onctl` against GitHub runners.
- `origin/ping`: A utility branch for testing network or service connectivity.

## Other
- `origin/domain`: Likely contains core domain models and business logic.
- `origin/jules_wip_...`: A work-in-progress branch belonging to a developer (Jules).

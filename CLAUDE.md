# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**onctl** is a multi-cloud VM management tool written in Go that supports AWS, Azure, GCP, and Hetzner. It provides a simple CLI interface to create, manage, and SSH into virtual machines across different cloud providers.

## Development Commands

### Building and Testing
```bash
# Build the binary
make build

# Run tests
make test
go test ./...

# Clean build artifacts
make clean

# Format code (automatically done during build)
go fmt ./...

# Tidy dependencies
go mod tidy
```

### Environment Setup
```bash
# Initialize onctl project (creates .onctl directory)
onctl init

# Set cloud provider (required for most operations)
export ONCTL_CLOUD=hetzner  # or aws, azure, gcp

# Enable debug logging
export ONCTL_LOG=DEBUG
```

## Architecture

### Core Components

- **main.go**: Entry point with logging configuration using hashicorp/logutils
- **cmd/**: Cobra CLI commands (root, create, destroy, list, ssh, network, etc.)
- **internal/cloud/**: Core domain models and interfaces
  - `CloudProviderInterface`: Main provider abstraction
  - `NetworkManager`: Network operations interface  
  - `Vm`, `Network`: Core data structures
- **internal/provider*/**: Cloud provider implementations
  - `provideraws/`, `providerazure/`, `providergcp/`, `providerhtz/`
- **internal/tools/**: Utilities for SSH, SCP, remote execution, cloud-init
- **internal/domain/**: Domain-specific logic (Cloudflare integration)

### Multi-Cloud Architecture

The application uses a provider pattern where each cloud platform implements the `CloudProviderInterface`:

- **Hetzner**: Uses `hcloud-go/v2` SDK
- **AWS**: Uses `aws-sdk-go` 
- **Azure**: Uses Azure SDK for Go v2
- **GCP**: Uses Google Cloud Go SDK

Provider selection is handled in `cmd/root.go:67-108` based on the `ONCTL_CLOUD` environment variable.

### Configuration

- Config files are stored in `.onctl/` directory (created by `onctl init`)
- Cloud provider credentials use each platform's standard authentication (AWS profiles, Azure CLI, GCP service accounts, Hetzner tokens)
- SSH key management is handled per-provider with automatic public key upload

### Key Features

- **VM Lifecycle**: Create, destroy, list VMs across providers
- **SSH Integration**: Direct SSH access with `onctl ssh <vm-name>`
- **Network Management**: Create and manage virtual networks (where supported)
- **Cloud-init Support**: Custom VM initialization scripts
- **Template System**: Ready-to-use configuration templates via onctl-templates repo
- **Jump Host Support**: SSH proxy capabilities for private networks

## Testing

- Unit tests are co-located with source files (`*_test.go`)
- Integration tests in `cmd/integration_test.go`
- Coverage tracking via `cmd/coverage_test.go` and `cmd/final_coverage_test.go`
- Test environment requires cloud provider credentials for integration tests

## Dependencies

Key external dependencies:
- **CLI**: `spf13/cobra` for command structure, `manifoldco/promptui` for interactive prompts
- **Cloud SDKs**: Provider-specific SDKs for each supported platform
- **SSH/Network**: `golang.org/x/crypto` for SSH, `pkg/sftp` for file transfers
- **Config**: `spf13/viper` for configuration management
- **Utilities**: `briandowns/spinner` for UI, `gofrs/uuid` for ID generation
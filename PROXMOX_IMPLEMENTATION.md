# Proxmox Implementation Summary

This document summarizes the Proxmox provider implementation for onctl.

## Files Created/Modified

### New Files

1. **internal/providerpxmx/common.go**
   - Proxmox client initialization
   - Handles API authentication using token-based auth
   - Supports self-signed certificates

2. **internal/cloud/proxmox.go**
   - Main provider implementation
   - Implements `CloudProviderInterface`
   - Methods: Deploy, Destroy, List, CreateSSHKey, GetByName, SSHInto

3. **internal/files/init/proxmox.yaml**
   - Default configuration template
   - Includes node, VM specs, storage, and network settings

4. **docs/PROXMOX.md**
   - Comprehensive user documentation
   - Setup instructions, examples, and troubleshooting

### Modified Files

1. **cmd/root.go**
   - Added "proxmox" to cloud provider list
   - Added import for `internal/providerpxmx`
   - Added Proxmox case to provider switch statement

2. **go.mod** (via go get)
   - Added dependency: `github.com/Telmate/proxmox-api-go`

## Key Features

### VM Management
- **Deploy**: Clone VMs from templates with custom configurations
- **Destroy**: Stop and delete VMs
- **List**: List all VMs tagged with "onctl"
- **SSH**: Connect to VMs via SSH

### Configuration Options
- Node selection
- VM ID management
- Template selection
- CPU cores and memory allocation
- Storage pool selection
- Network bridge configuration
- Cloud-init support
- SSH key injection

### Implementation Details

1. **VM Cloning**
   - Clones from existing Proxmox templates
   - Full clones for isolated VMs
   - Automatic configuration after cloning

2. **Tagging System**
   - All VMs tagged with "onctl"
   - Enables filtering onctl-managed VMs
   - Prevents interference with other VMs

3. **Network Configuration**
   - Configurable bridge networking
   - Automatic IP address detection
   - Support for both public and private IPs

4. **SSH Integration**
   - Public key injection via cloud-init
   - Automatic SSH key management
   - Seamless SSH connection

## Environment Variables

Required environment variables:
```bash
PROXMOX_API_URL      # Proxmox API endpoint
PROXMOX_TOKEN_ID     # API token ID
PROXMOX_SECRET       # API token secret
ONCTL_CLOUD          # Set to "proxmox"
```

## API Dependencies

- `github.com/Telmate/proxmox-api-go/proxmox` - Official Proxmox Go API client

## Compatibility

- Proxmox VE 7.x and 8.x
- Supports QEMU/KVM virtual machines
- Cloud-init compatible templates

## Usage Example

```bash
# Setup
export PROXMOX_API_URL="https://pve.example.com:8006/api2/json"
export PROXMOX_TOKEN_ID="root@pam!onctl"
export PROXMOX_SECRET="your-secret"
export ONCTL_CLOUD="proxmox"

# Initialize
onctl init

# Create VM
onctl create -n my-proxmox-vm

# List VMs
onctl ls

# SSH into VM
onctl ssh my-proxmox-vm

# Destroy VM
onctl destroy my-proxmox-vm
```

## Implementation Pattern

The Proxmox implementation follows the same pattern as other cloud providers:

1. **Provider struct** with API client
2. **CloudProviderInterface** implementation
3. **Configuration via Viper** from YAML files
4. **VM struct mapping** for consistent API
5. **Error handling** with proper logging
6. **Context-based API calls** for timeouts

## Testing

The implementation has been:
- ✅ Successfully compiled
- ✅ All methods implement CloudProviderInterface
- ✅ Compatible with existing onctl architecture
- ✅ Documented with examples

## Future Enhancements

Potential improvements:
- LXC container support
- Storage management
- Snapshot support
- Resource pool management
- Multi-node cluster support
- Custom cloud-init configurations
- Network VLAN support

## Notes

- Self-signed certificates are accepted by default (configurable)
- VM IDs must be unique on the Proxmox node
- Templates must be prepared with cloud-init
- API token requires appropriate permissions
- VMs are tagged for easy management and cleanup

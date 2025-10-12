# Proxmox Provider for onctl

This guide explains how to use onctl with Proxmox Virtual Environment.

## Prerequisites

1. A running Proxmox VE server (tested with Proxmox VE 7.x and 8.x)
2. API token or username/password credentials
3. A VM template configured with cloud-init support (recommended)

## Setup

### 1. Create API Token in Proxmox

1. Log in to your Proxmox web interface
2. Go to Datacenter → Permissions → API Tokens
3. Click "Add" to create a new API token
4. Note down the Token ID and Secret

Alternatively, you can use username/password authentication.

### 2. Set Environment Variables

```bash
export PROXMOX_API_URL="https://your-proxmox-server:8006/api2/json"
export PROXMOX_TOKEN_ID="your-token-id"
export PROXMOX_SECRET="your-secret"
export ONCTL_CLOUD="proxmox"
```

### 3. Initialize onctl

```bash
onctl init
```

This will create a `.onctl` directory with a `proxmox.yaml` configuration file.

### 4. Configure proxmox.yaml

Edit `~/.onctl/proxmox.yaml` or `./.onctl/proxmox.yaml`:

```yaml
proxmox:
  node: pve                          # Proxmox node name
  vm:
    id: 100                          # Starting VM ID (will be used for cloning)
    template: ubuntu-22.04-template  # Template VM name
    username: root                   # Default username for SSH
    cores: 2                         # Number of CPU cores
    memory: 2048                     # Memory in MB
    storage: local-lvm               # Storage pool for cloned VMs
    network_bridge: vmbr0            # Network bridge
```

## Creating a VM Template

For best results, create a VM template with cloud-init support:

1. Create a new VM in Proxmox
2. Install your preferred OS (Ubuntu, Debian, etc.)
3. Install cloud-init: `apt-get install cloud-init`
4. Install QEMU guest agent: `apt-get install qemu-guest-agent`
5. Clean up and prepare the VM for template conversion
6. Convert to template in Proxmox UI

## Usage Examples

### Create a VM

```bash
# Create a basic VM
onctl create -n myvm

# Create a VM with Docker installed
onctl create -n myvm -a docker/docker.sh
```

### List VMs

```bash
onctl ls
```

### SSH into a VM

```bash
onctl ssh myvm
```

### Destroy a VM

```bash
onctl destroy myvm
```

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `node` | Proxmox node name | pve |
| `vm.id` | Starting VM ID for clones | 100 |
| `vm.template` | Template VM name to clone from | ubuntu-22.04-template |
| `vm.username` | Default SSH username | root |
| `vm.cores` | Number of CPU cores | 2 |
| `vm.memory` | Memory in MB | 2048 |
| `vm.storage` | Storage pool name | local-lvm |
| `vm.network_bridge` | Network bridge | vmbr0 |

## Important Notes

1. **VM IDs**: The specified `vm.id` will be used for cloning. Make sure it doesn't conflict with existing VMs.

2. **Templates**: The template VM must exist on the specified Proxmox node before you can create VMs.

3. **Networking**: VMs will use the specified network bridge. Ensure it's properly configured in your Proxmox setup.

4. **SSH Keys**: onctl uses SSH keys for authentication. The public key is automatically injected into VMs during creation if cloud-init is configured.

5. **Tags**: All VMs created by onctl are tagged with "onctl" for easy identification and management.

6. **Self-Signed Certificates**: The implementation currently accepts self-signed SSL certificates. For production use, consider using valid certificates.

## Troubleshooting

### VM Creation Fails

- Verify the template exists: Check in Proxmox UI under the specified node
- Check VM ID: Ensure the ID isn't already in use
- Verify storage: Ensure the storage pool has enough space
- Check network bridge: Verify the bridge name is correct

### Cannot Connect to Proxmox API

- Verify `PROXMOX_API_URL` is correct (should include `/api2/json`)
- Check API token has necessary permissions
- Ensure Proxmox server is accessible from your machine
- Verify firewall settings allow port 8006

### SSH Connection Issues

- Ensure cloud-init is properly configured in the template
- Check if the VM has obtained an IP address
- Verify SSH key was injected correctly
- Check network connectivity to the VM

## API Permissions

The API token/user needs the following permissions:

- VM.Allocate
- VM.Clone
- VM.Config.Disk
- VM.Config.CPU
- VM.Config.Memory
- VM.Config.Network
- VM.Monitor
- VM.PowerMgmt
- Datastore.AllocateSpace

## Example Workflow

```bash
# Set up environment
export PROXMOX_API_URL="https://pve.example.com:8006/api2/json"
export PROXMOX_TOKEN_ID="root@pam!onctl"
export PROXMOX_SECRET="your-secret-here"
export ONCTL_CLOUD="proxmox"

# Initialize
onctl init

# Create a VM
onctl create -n test-vm

# List all VMs
onctl ls

# SSH into the VM
onctl ssh test-vm

# Clean up
onctl destroy test-vm

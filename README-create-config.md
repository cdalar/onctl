# Onctl Create Command Configuration

This document explains how to use the `example-create-config.yaml` file with the `onctl create` command.

## Usage

Create a VM using a configuration file:

```bash
onctl create -f example-create-config.yaml
```

You can override individual settings using command-line flags:

```bash
onctl create -f example-create-config.yaml -n my-custom-name --domain mydomain.com
```

## Configuration File Structure

The configuration file supports all the options available through command-line flags:

| YAML Field | CLI Flag | Description | Required |
|------------|----------|-------------|----------|
| `publicKeyFile` | `-k, --publicKey` | Path to SSH public key file | No (defaults to `~/.ssh/id_rsa.pub`) |
| `applyFiles` | `-a, --apply-file` | List of bash scripts to run on VM | No |
| `dotEnvFile` | `--dot-env` | Path to .env file for environment variables | No |
| `variables` | `-e, --vars` | Environment variables as key=value pairs | No |
| `vm.name` | `-n, --name` | VM name | No (auto-generated if not specified) |
| `vm.sshPort` | `-p, --ssh-port` | SSH port number | No (defaults to 22) |
| `vm.cloudInitFile` | `-i, --cloud-init` | Cloud-init configuration file | No |
| `domain` | `--domain` | Domain name for Cloudflare integration | No |
| `downloadFiles` | `-d, --download` | Files to download from VM after creation | No |
| `uploadFiles` | `-u, --upload` | Files to upload to VM before scripts | No |

## File Upload/Download Format

### Upload Files
Files can be uploaded to the VM before running scripts:

```yaml
uploadFiles:
  - "local-file.txt"                    # Upload to same path on remote
  - "local-config.yaml:/etc/app/config.yaml"  # Upload to specific remote path
```

### Download Files
Files can be downloaded from the VM after configuration:

```yaml
downloadFiles:
  - "/var/log/app.log"      # Download from remote path (saved locally with same name)
  - "/etc/config.yaml"      # Another file to download
```

## Environment Variables

For domain configuration, set these environment variables:

```bash
export CLOUDFLARE_API_TOKEN="your-api-token"
export CLOUDFLARE_ZONE_ID="your-zone-id"
```

## Example Workflows

### Web Application Deployment

```yaml
publicKeyFile: "~/.ssh/production.pub"
applyFiles:
  - "scripts/install-docker.sh"
  - "scripts/deploy-app.sh"
variables:
  - "APP_ENV=production"
  - "PORT=8080"
vm:
  name: "web-prod-01"
  sshPort: 22
  cloudInitFile: "cloud-init/web-server.yaml"
domain: "api.example.com"
uploadFiles:
  - "docker-compose.yml:/home/ubuntu/docker-compose.yml"
  - ".env:/home/ubuntu/.env"
```

### Database Server

```yaml
applyFiles:
  - "scripts/install-postgresql.sh"
  - "scripts/setup-database.sh"
dotEnvFile: ".env.db"
vm:
  name: "db-server"
  cloudInitFile: "cloud-init/database.yaml"
downloadFiles:
  - "/var/log/postgresql/postgresql.log"
```

## Notes

- All paths can be absolute or relative to the current working directory
- SSH keys must exist and be readable
- Cloud-init files should follow standard cloud-init YAML format
- Scripts in `applyFiles` are executed in order on the remote VM
- Environment variables from `dotEnvFile` and `variables` are merged, with `variables` taking precedence
- The `configFile` field can reference other config files for advanced scenarios (typically left empty)

## See Also

- `onctl create --help` for command-line usage
- Provider-specific configuration files in `internal/files/init/`
- Cloud-init documentation for cloudInitFile format

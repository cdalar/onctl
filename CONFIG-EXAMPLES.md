# onctl Configuration File Examples

This directory contains example configuration files for the `onctl up -f` command. These files allow you to define all your VM deployment settings in a single YAML file instead of using multiple command-line flags.

## Quick Start

1. **Copy an example configuration file:**
   ```bash
   cp minimal-config.yaml my-config.yaml
   ```

2. **Edit the configuration file:**
   ```bash
   nano my-config.yaml
   ```

3. **Deploy using the configuration file:**
   ```bash
   onctl up -f my-config.yaml
   ```

## Available Configuration Files

### `minimal-config.yaml`
The simplest possible configuration - just a VM name and one script to run.

### `example-config.yaml`
A comprehensive example showing all available configuration options with comments.

### `sample-config.yaml`
An extensive reference with multiple use case examples including:
- Simple web server setup
- Development environment
- Database server
- CI/CD agent
- Kubernetes cluster node
- Monitoring server
- File server with custom domain
- Game server
- Backup server
- Load balancer

## Configuration Options

### Required Fields
- `vm.name`: The name of your VM

### Optional Fields

#### VM Configuration
- `vm.sshPort`: SSH port (default: 22)
- `vm.cloudInitFile`: Path to cloud-init file
- `vm.jumpHost`: Jump host for SSH tunneling

#### SSH Configuration
- `publicKeyFile`: Path to public key file (default: ~/.ssh/id_rsa.pub)

#### Domain Configuration
- `domain`: Request a domain name for the VM
  - Requires `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ZONE_ID` environment variables

#### Scripts and Files
- `applyFiles`: Array of bash scripts to run on the remote VM
- `downloadFiles`: Array of files to download from the remote VM
- `uploadFiles`: Array of files to upload to the remote VM (format: "local:remote")

#### Environment Variables
- `variables`: Array of environment variables passed to scripts
- `dotEnvFile`: Path to .env file (alternative to variables array)

## Examples

### Basic Web Server
```yaml
vm:
  name: "web-server"
  sshPort: 22

applyFiles:
  - "nginx/nginx-setup.sh"
  - "ssl/letsencrypt.sh"

variables:
  - "DOMAIN=example.com"
  - "EMAIL=admin@example.com"
```

### Development Environment
```yaml
vm:
  name: "dev-env"
  sshPort: 22

applyFiles:
  - "docker/docker.sh"
  - "nodejs/nodejs.sh"
  - "git/git-setup.sh"

variables:
  - "NODE_ENV=development"
  - "GIT_USER=developer"
```

### Using .env File
```yaml
vm:
  name: "ci-agent"
  sshPort: 22

applyFiles:
  - "azure/agent-pool.sh"

dotEnvFile: ".env.azure"
```

Where `.env.azure` contains:
```
TOKEN=your_pat_token
AGENT_POOL_NAME=your_pool_name
URL=https://dev.azure.com/your_org
```

## File Upload/Download Examples

### Upload Files
```yaml
uploadFiles:
  - "configs/nginx.conf:/etc/nginx/nginx.conf"
  - "scripts/backup.sh:/home/user/backup.sh"
  - "data/app-data.json:/var/www/app/data.json"
```

### Download Files
```yaml
downloadFiles:
  - "/var/log/nginx/access.log"
  - "/home/user/app.log"
  - "/etc/nginx/nginx.conf"
```

## Environment Variables

You can pass environment variables to your scripts in two ways:

### Method 1: Using variables array
```yaml
variables:
  - "APP_ENV=production"
  - "DB_HOST=localhost"
  - "API_KEY=your-secret-key"
  - "DEBUG=false"
```

### Method 2: Using .env file
```yaml
dotEnvFile: ".env.production"
```

## Domain Configuration

To automatically assign a domain name to your VM:

1. Set up Cloudflare environment variables:
   ```bash
   export CLOUDFLARE_API_TOKEN="your-api-token"
   export CLOUDFLARE_ZONE_ID="your-zone-id"
   ```

2. Add domain to your configuration:
   ```yaml
   domain: "my-vm.example.com"
   ```

## Best Practices

1. **Use descriptive VM names** that indicate the purpose
2. **Group related scripts** in the applyFiles array
3. **Use environment variables** for configuration instead of hardcoding values
4. **Test with minimal-config.yaml** first before using complex configurations
5. **Keep sensitive data in .env files** and add them to .gitignore
6. **Use version control** for your configuration files
7. **Document your configurations** with comments explaining the purpose

## Troubleshooting

### Common Issues

1. **Configuration file not found:**
   - Ensure the file path is correct
   - Use absolute paths if needed

2. **Script files not found:**
   - Check that script paths are relative to your current directory
   - Ensure scripts exist and are executable

3. **Environment variables not working:**
   - Check variable syntax (KEY=VALUE)
   - Ensure .env file exists if using dotEnvFile

4. **Domain configuration fails:**
   - Verify Cloudflare environment variables are set
   - Check that the domain is managed by your Cloudflare account

### Debug Mode

Add debug logging to see what's happening:
```bash
onctl up -f my-config.yaml --debug
```

## More Examples

For more detailed examples and use cases, see `sample-config.yaml` which includes configurations for:
- Web servers
- Development environments
- Database servers
- CI/CD agents
- Kubernetes nodes
- Monitoring servers
- Game servers
- Backup servers
- Load balancers

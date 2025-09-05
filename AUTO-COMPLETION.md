# onctl Auto-Completion Guide

onctl now supports comprehensive auto-completion for all major shells, making it much easier to use the CLI with tab completion for server names, network names, and template names.

## 🚀 Quick Setup

### Bash
```bash
# Load completion for current session
source <(onctl completion bash)

# Install permanently (Linux)
sudo onctl completion bash > /etc/bash_completion.d/onctl

# Install permanently (macOS)
onctl completion bash > /usr/local/etc/bash_completion.d/onctl
```

### Zsh
```bash
# Load completion for current session
source <(onctl completion zsh)

# Install permanently
onctl completion zsh > "${fpath[1]}/_onctl"

# Enable completion in your shell (if not already enabled)
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

### Fish
```bash
# Load completion for current session
onctl completion fish | source

# Install permanently
onctl completion fish > ~/.config/fish/completions/onctl.fish
```

### PowerShell
```powershell
# Load completion for current session
onctl completion powershell | Out-String | Invoke-Expression

# Install permanently
onctl completion powershell > onctl.ps1
# Then source this file from your PowerShell profile
```

## 📋 Supported Auto-Completion

### ✅ Commands with Auto-Completion

#### 1. **SSH Command** - `onctl ssh <TAB>`
- **Completes**: VM/Server names from your cloud provider
- **Example**: `onctl ssh te<TAB>` → `onctl ssh test-vm`

```bash
onctl ssh <TAB>
# Shows: test-vm, web-server, db-server, etc.
```

#### 2. **Destroy Command** - `onctl destroy <TAB>`
- **Completes**: VM/Server names from your cloud provider
- **Example**: `onctl destroy te<TAB>` → `onctl destroy test-vm`

```bash
onctl destroy <TAB>
# Shows: test-vm, web-server, db-server, etc.
```

#### 3. **Network Delete Command** - `onctl network delete <TAB>`
- **Completes**: Network names from your cloud provider
- **Example**: `onctl network delete my<TAB>` → `onctl network delete my-network`

```bash
onctl network delete <TAB>
# Shows: my-network, vpc-123, subnet-456, etc.
```

#### 4. **VM Network Attach** - `onctl vm attach <TAB>`
- **Completes**: VM names and network names based on context
- **Example**: `onctl vm attach --vm test-vm --network <TAB>`

```bash
onctl vm attach --vm <TAB>
# Shows: test-vm, web-server, db-server, etc.

onctl vm attach --vm test-vm --network <TAB>
# Shows: my-network, vpc-123, subnet-456, etc.
```

#### 5. **VM Network Detach** - `onctl vm detach <TAB>`
- **Completes**: VM names and network names based on context
- **Example**: `onctl vm detach --vm test-vm --network <TAB>`

```bash
onctl vm detach --vm <TAB>
# Shows: test-vm, web-server, db-server, etc.

onctl vm detach --vm test-vm --network <TAB>
# Shows: my-network, vpc-123, subnet-456, etc.
```

#### 6. **Templates Describe** - `onctl templates describe <TAB>`
- **Completes**: Template names from the template index
- **Example**: `onctl templates describe az<TAB>` → `onctl templates describe azure`

```bash
onctl templates describe <TAB>
# Shows: azure, docker, nginx, kubernetes, etc.
```

## 🔧 How It Works

### Dynamic Completion
The auto-completion system dynamically fetches data from your cloud provider:

1. **VM Names**: Retrieved from `provider.List()` - shows all your VMs
2. **Network Names**: Retrieved from `networkManager.List()` - shows all your networks  
3. **Template Names**: Retrieved from the template index at `https://templates.onctl.com/index.yaml`

### Smart Context-Aware Completion
Some commands provide context-aware completion:

- **VM Network Commands**: Complete VM names first, then network names based on the `--vm` flag
- **Template Commands**: Complete template names from the remote template index

### Error Handling
- If the cloud provider is unreachable, completion gracefully fails
- If template index is unavailable, completion falls back gracefully
- All completion functions include proper error handling

## 🎯 Usage Examples

### Basic VM Operations
```bash
# SSH into a VM with auto-completion
onctl ssh te<TAB>          # Completes to: test-vm
onctl ssh test-vm

# Destroy a VM with auto-completion  
onctl destroy we<TAB>       # Completes to: web-server
onctl destroy web-server
```

### Network Operations
```bash
# Delete a network with auto-completion
onctl network delete my<TAB>    # Completes to: my-network
onctl network delete my-network
```

### VM Network Management
```bash
# Attach network to VM with auto-completion
onctl vm attach --vm te<TAB> --network my<TAB>
# Completes to: onctl vm attach --vm test-vm --network my-network
```

### Template Operations
```bash
# Describe a template with auto-completion
onctl templates describe az<TAB>    # Completes to: azure
onctl templates describe azure
```

## 🛠️ Technical Details

### Implementation
- Uses Cobra's `ValidArgsFunction` for dynamic completion
- Fetches data from cloud providers in real-time
- Supports all major shells: Bash, Zsh, Fish, PowerShell
- Includes proper error handling and fallbacks

### Performance
- Completion data is fetched on-demand (not cached)
- Lightweight and fast for typical use cases
- Graceful degradation when providers are unavailable

### Shell Compatibility
- **Bash**: Full support with `__onctl_init_completion`
- **Zsh**: Full support with `_onctl` completion function
- **Fish**: Full support with native fish completion
- **PowerShell**: Full support with PowerShell completion

## 🔍 Troubleshooting

### Completion Not Working

1. **Check if completion is installed**:
   ```bash
   # For bash
   type _onctl
   
   # For zsh  
   type _onctl
   ```

2. **Reload your shell**:
   ```bash
   # Reload bash completion
   source ~/.bashrc
   
   # Reload zsh completion
   source ~/.zshrc
   ```

3. **Check shell completion is enabled**:
   ```bash
   # For zsh, ensure this is in your .zshrc
   autoload -U compinit; compinit
   ```

### Cloud Provider Issues

If completion shows no results:

1. **Check cloud provider configuration**:
   ```bash
   echo $ONCTL_CLOUD
   onctl ls  # Should show your VMs
   ```

2. **Verify network connectivity**:
   ```bash
   # Test if you can reach your cloud provider
   onctl ls
   ```

### Template Completion Issues

If template completion doesn't work:

1. **Check network connectivity**:
   ```bash
   curl -s https://templates.onctl.com/index.yaml
   ```

2. **Use local index file**:
   ```bash
   onctl templates list --file local-index.yaml
   onctl templates describe azure --file local-index.yaml
   ```

## 📚 Advanced Usage

### Custom Completion Scripts

You can create custom completion scripts for specific use cases:

```bash
# Create a custom completion for your team's VMs
_onctl_custom() {
    local vms=("prod-web" "staging-web" "dev-web")
    COMPREPLY=($(compgen -W "${vms[*]}" -- "${COMP_WORDS[COMP_CWORD]}"))
}
complete -F _onctl_custom onctl
```

### Integration with Other Tools

The completion system integrates well with other CLI tools:

```bash
# Use with fzf for fuzzy completion
onctl ssh $(onctl ls --output json | jq -r '.[].name' | fzf)

# Use with other completion systems
complete -F _onctl onctl
```

## 🎉 Benefits

- **Faster CLI usage**: No need to remember exact VM names
- **Reduced errors**: Prevents typos in VM/network names
- **Better UX**: Professional CLI experience
- **Cross-platform**: Works on all major operating systems
- **Dynamic**: Always shows current resources from your cloud provider

## 🔄 Updates

The auto-completion system is automatically updated when you update onctl. New commands and completion features are added regularly.

For the latest completion features, always use the latest version of onctl:

```bash
# Update onctl to get latest completion features
# (Update method depends on how you installed onctl)
```

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(completionCmd)
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `To load completions:

Bash:
  $ source <(onctl completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ onctl completion bash > /etc/bash_completion.d/onctl
  # macOS:
  $ onctl completion bash > /usr/local/etc/bash_completion.d/onctl

Zsh:
  # If shell completion is not already enabled in your environment you will need
  # to enable it.  You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ onctl completion zsh > "${fpath[1]}/_onctl"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ onctl completion fish | source

  # To load completions for each session, execute once:
  $ onctl completion fish > ~/.config/fish/completions/onctl.fish

PowerShell:
  PS> onctl completion powershell | Out-String | Invoke-Expression

  # To load completions for each session, execute once:
  PS> onctl completion powershell > onctl.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactValidArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			err := rootCmd.GenBashCompletion(os.Stdout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error generating bash completion: %v\n", err)
				os.Exit(1)
			}
		case "zsh":
			err := rootCmd.GenZshCompletion(os.Stdout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error generating zsh completion: %v\n", err)
				os.Exit(1)
			}
		case "fish":
			err := rootCmd.GenFishCompletion(os.Stdout, true)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error generating fish completion: %v\n", err)
				os.Exit(1)
			}
		case "powershell":
			err := rootCmd.GenPowerShellCompletion(os.Stdout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error generating powershell completion: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

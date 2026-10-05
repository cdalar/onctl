package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/cdalar/onctl/internal/providerboxes"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// boxesDashboardURL is where personal tokens are made.
const boxesDashboardURL = "https://boxctl.io"

var loginAPIURL string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to hosted boxes (-p boxes) with a personal token",
	Long: `Saves a personal token for the hosted boxes service to ~/.onctl/boxes.json
(readable only by you). Create the token in the dashboard at ` + boxesDashboardURL + `.

The token is read from a hidden prompt, or from stdin when piped. It's
checked against the service before it's saved. $` + providerboxes.TokenEnv + `
overrides the saved token without logging in, for CI.

If you used boxctl, its saved token (~/.boxctl/config.json) already works:
onctl reads it until you log in here.`,
	Example: `  onctl login
  pbpaste | onctl login`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiURL := loginAPIURL
		if apiURL == "" {
			apiURL = providerboxes.DefaultAPIURL
		}
		token, err := readBoxesToken(os.Stdin)
		if err != nil {
			return err
		}
		if _, err := providerboxes.New(apiURL, token).List(cmd.Context()); err != nil {
			return fmt.Errorf("that token didn't work against %s: %w", apiURL, err)
		}
		if err := providerboxes.SaveCredentials(&providerboxes.Credentials{APIURL: apiURL, Token: token}); err != nil {
			return fmt.Errorf("saving the token: %w", err)
		}
		fmt.Println("Logged in. Try: onctl ls -p boxes")
		printPluginHint()
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Forget the saved boxes token",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := providerboxes.ClearCredentials(); err != nil {
			return err
		}
		fmt.Println("Logged out.")
		return nil
	},
}

// readBoxesToken reads the token from a hidden prompt on a terminal, or
// the first line of piped stdin.
func readBoxesToken(in *os.File) (string, error) {
	var raw string
	if term.IsTerminal(int(in.Fd())) {
		fmt.Fprintf(os.Stderr, "Paste your boxes token (input hidden; create one at %s): ", boxesDashboardURL)
		b, err := term.ReadPassword(int(in.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("reading token: %w", err)
		}
		raw = string(b)
	} else {
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("reading token from stdin: %w", err)
		}
		raw = line
	}
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("no token given: paste it at the prompt, or pipe it in (pbpaste | onctl login)")
	}
	return token, nil
}

func init() {
	loginCmd.Flags().StringVar(&loginAPIURL, "api-url", "", "boxes service URL (default "+providerboxes.DefaultAPIURL+")")
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
}

// printPluginHint suggests the Claude Code plugin to someone who has
// Claude Code -- but not inside it, where the line would only be noise in
// Claude's tool output. (Claude Code's own <claude-code-hint> install
// prompt only works for plugins in Anthropic's official marketplaces.)
func printPluginHint() {
	if os.Getenv("CLAUDECODE") != "" {
		return
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return
	}
	fmt.Println("\nUsing Claude Code? Have it run its Bash commands on a box:")
	fmt.Println("  claude plugin marketplace add cdalar/onctl && claude plugin install onctl@onctl")
}

package main

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for tsuku.

To load completions:

Bash:
  $ source <(tsuku completion bash)
  # Or, to load completions for each session:
  $ tsuku completion bash > ~/.bash_completion.d/tsuku

Zsh:
  # If shell completion is not already enabled in your environment:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  $ source <(tsuku completion zsh)
  # Or, to load completions for each session:
  $ tsuku completion zsh > "${fpath[1]}/_tsuku"

Fish:
  $ tsuku completion fish | source
  # Or, to load completions for each session:
  $ tsuku completion fish > ~/.config/fish/completions/tsuku.fish
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			_ = cmd.Root().GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			_ = cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			_ = cmd.Root().GenFishCompletion(os.Stdout, true)
		}
	},
}

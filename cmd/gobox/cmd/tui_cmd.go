package cmd

import (
	"fmt"
	"os"

	"gobox/internal/core"
	"gobox/internal/tui"

	"github.com/spf13/cobra"
)

// tuiCmd launches the GoBox interactive TUI.
var tuiCmd = &cobra.Command{
	Use:   "tui [markdown_file]",
	Short: "Launch the GoBox interactive TUI",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		markdownFile := args[0]
		stateMgr := core.NewFileStateStore(".gobox_state.json")
		states, _ := stateMgr.Load()
		if err := tui.Run(markdownFile, stateMgr, states); err != nil {
			fmt.Println("Error running TUI:", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
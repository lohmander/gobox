package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"gobox/internal/core" // For state store initialization
	"gobox/internal/tui"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gobox [markdown_file]",
	Short: "A tiny CLI tool for timeboxing tasks in Markdown files with Git integration",
	Long: `gobox parses a markdown file, starts a timer for the next unchecked task with a timebox,
updates the markdown upon completion with a checkmark and Git commits.`,
	Args: cobra.ExactArgs(1), // Expect exactly one argument: the markdown file path
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

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Any global flags or initializations can go here.
	// rootCmd.AddCommand(tuiCmd) // Will be added in tui_cmd.go
}

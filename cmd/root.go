package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tfs",
	Short: "Terraform Plan Analyzer",
	Long:  `A CLI tool to analyze Terraform plans. Use subcommands to generate reports or view in TUI.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

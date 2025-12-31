package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"log"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/bernard-sh/tfs/internal/ui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui <plan.binary>",
	Short: "Show terraform plan on TUI mode",
	Long:  `Show terraform plan on TUI mode`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {		
		filename := args[0]

		// Validations
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			log.Fatalf("File does not exist: %s", filename)
		}

		// Get JSON Content
		// Try terraform show -json first
		tfCmd := exec.Command("terraform", "show", "-json", filename)
		output, err := tfCmd.Output()
		if err != nil {
			// Fallback: Read file directly if it's already JSON
			raw, readErr := os.ReadFile(filename)
			if readErr != nil {
				log.Fatalf("Failed to run terraform output: %v, and failed to read file: %v", err, readErr)
			}
			output = raw
		}
		
		jsonContent := string(output)

		// 2. Parse
		var plan ui.TfPlan
		dec := json.NewDecoder(strings.NewReader(jsonContent))
		dec.UseNumber()
		if err := dec.Decode(&plan); err != nil {
			log.Fatalf("Failed to parse plan JSON: %v", err)
		}

		// 3. Start TUI

		model, err := ui.InitialModel(jsonContent)
		if err != nil {
			log.Fatalf("Error initializing model: %v\nPossible causes:\n1. Input is not valid JSON and 'terraform show -json' failed.\n2. JSON structure mismatch.", err)
		}

		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Display error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
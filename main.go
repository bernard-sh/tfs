package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"log"
	"os"
	"os/exec"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// --- 1. DATA MODELS (From previous conversation) ---

type TfPlan struct {
	ResourceChanges []ResourceChange `json:"resource_changes"`
}

type ResourceChange struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Change  Change `json:"change"`
}

type Change struct {
	Actions      []string               `json:"actions"`
	Before       map[string]interface{} `json:"before"`
	After        map[string]interface{} `json:"after"`
	AfterUnknown map[string]interface{} `json:"after_unknown"`
}

// Helper to pretty-print attributes with recursive diff style
func renderDiff(rc ResourceChange) string {
	var s strings.Builder
	
	// Styles
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00AF00")) // Green
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D70000")) // Red
	modStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAF00")) // Yellow
	repStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#AE00FF")) // Purple for Replace

	// Helper to format a value for display (raw strings, no color)
	var formatValue func(v interface{}, indent int) string
	formatValue = func(v interface{}, indent int) string {
		if v == nil {
			return "null"
		}
		switch val := v.(type) {
		case map[string]interface{}:
			var sb strings.Builder
			sb.WriteString("{\n")
			keys := make([]string, 0, len(val))
			for k := range val { keys = append(keys, k) }
			sort.Strings(keys)
			// Indent step is 4 spaces to match Terraform plan format provided
			padding := strings.Repeat(" ", indent+4)
			for _, k := range keys {
				sb.WriteString(fmt.Sprintf("%s%s = %s\n", padding, k, formatValue(val[k], indent+4)))
			}
			sb.WriteString(strings.Repeat(" ", indent) + "}")
			return sb.String()
		case []interface{}:
			if len(val) == 0 {
				return "[]"
			}
			var sb strings.Builder
			sb.WriteString("[\n")
			padding := strings.Repeat(" ", indent+4)
			for _, item := range val {
				sb.WriteString(fmt.Sprintf("%s%s,\n", padding, formatValue(item, indent+4)))
			}
			sb.WriteString(strings.Repeat(" ", indent) + "]")
			return sb.String()
		case string:
			return fmt.Sprintf("%q", val)
		default:
			return fmt.Sprintf("%v", val)
		}
	}

	// Recursive diff function that returns COLORED string blocks
	var stringifyDiff func(key string, valBefore, valAfter interface{}, unknown interface{}, indent int) string
	stringifyDiff = func(key string, valBefore, valAfter interface{}, unknown interface{}, indent int) string {
		var sb strings.Builder
		padding := strings.Repeat(" ", indent)

		// Check if "known after apply"
		isUnknown := false
		if b, ok := unknown.(bool); ok && b {
			isUnknown = true
		}

		// 1. ADDITION (+ key = value)
		if valBefore == nil && (valAfter != nil || isUnknown) {
			valStr := "(known after apply)"
			if !isUnknown {
				valStr = formatValue(valAfter, indent)
			}
			
			// If formatValue is multi-line, it returns uncolored string. We wrap the preamble.
			// IMPORTANT: Do NOT include \n in the Render call to avoid staircase effect
			rawLine := fmt.Sprintf("%s+ %s = %s", padding, key, valStr)
			return addStyle.Render(rawLine) + "\n"
		}

		// 2. DELETION (- key = value)
		if valBefore != nil && valAfter == nil && !isUnknown {
			valStr := formatValue(valBefore, indent)
			rawLine := fmt.Sprintf("%s- %s = %s", padding, key, valStr)
			return delStyle.Render(rawLine) + "\n"
		}

		// 3. MODIFICATION or UNCHANGED
		// Handle Maps recursively
		mapBefore, isMapBefore := valBefore.(map[string]interface{})
		mapAfter, isMapAfter := valAfter.(map[string]interface{})
		
		if isMapBefore && isMapAfter {
			// Header: ~ key = {
			headerRaw := fmt.Sprintf("%s~ %s = {", padding, key)
			sb.WriteString(modStyle.Render(headerRaw) + "\n")
			
			// Union of keys
			seen := make(map[string]bool)
			for k := range mapBefore { seen[k] = true }
			for k := range mapAfter { seen[k] = true }
			
			allKeys := make([]string, 0, len(seen))
			for k := range seen { allKeys = append(allKeys, k) }
			sort.Strings(allKeys)

			for _, k := range allKeys {
				var vB, vA interface{}
				if v, ok := mapBefore[k]; ok { vB = v }
				if v, ok := mapAfter[k]; ok { vA = v }
				
				// Recurse (inner lines will be colored themselves)
				sb.WriteString(stringifyDiff(k, vB, vA, nil, indent+4))
			}
			
			// Footer: }
			footerRaw := fmt.Sprintf("%s}", padding)
			sb.WriteString(modStyle.Render(footerRaw) + "\n")
			
			return sb.String()
		}

		// Scalar Update
		sBefore := formatValue(valBefore, indent)
		sAfter := "(known after apply)"
		if !isUnknown {
			sAfter = formatValue(valAfter, indent)
		}

		if sBefore != sAfter {
			rawLine := fmt.Sprintf("%s~ %s = %s -> %s", padding, key, sBefore, sAfter)
			return modStyle.Render(rawLine) + "\n"
		} else {
			return "" 
		}
	}

	// Main execution based on Action - Check for replace first
	isReplace := false
	action := rc.Change.Actions[0]
	if len(rc.Change.Actions) > 1 && rc.Change.Actions[0] == "delete" && rc.Change.Actions[1] == "create" {
		isReplace = true
		action = "replace"
	}

	// Styles for resource block header
	// e.g. # type.name will be created
	headerLine := fmt.Sprintf("# %s.%s will be %sed", rc.Type, rc.Name, action)
	if action == "update" {
		headerLine = fmt.Sprintf("# %s.%s will be updated in-place", rc.Type, rc.Name)
	} else if isReplace {
		headerLine = fmt.Sprintf("# %s.%s must be replaced", rc.Type, rc.Name)
	}
	s.WriteString(lipgloss.NewStyle().Bold(true).Render(headerLine) + "\n")

	// e.g. + resource "type" "name" {
	// User snippet shows 2 spaces indent for resource line.
	
	symbol := getSymbol(action)
	
	// Open Resource Block
	// "  + resource"
	resourceLine := fmt.Sprintf("  %s resource %q %q {", symbol, rc.Type, rc.Name)
	
	// Apply color to the resource opening line (Styled without trailing newline)
	if action == "create" {
		s.WriteString(addStyle.Render(resourceLine) + "\n")
	} else if action == "delete" {
		s.WriteString(delStyle.Render(resourceLine) + "\n")
	} else if action == "update" {
		s.WriteString(modStyle.Render(resourceLine) + "\n")
	} else if isReplace {
		s.WriteString(repStyle.Render(resourceLine) + "\n")
	} else {
		s.WriteString(resourceLine + "\n")
	}

	// Render Attributes
	// Collect top level keys
	seen := make(map[string]bool)
	if rc.Change.Before != nil {
		for k := range rc.Change.Before { seen[k] = true }
	}
	if rc.Change.After != nil {
		for k := range rc.Change.After { seen[k] = true }
	}
	for k := range rc.Change.AfterUnknown { seen[k] = true }

	allKeys := make([]string, 0, len(seen))
	for k := range seen { 
		if k == "id" { continue } 
		allKeys = append(allKeys, k)
	}
	sort.Strings(allKeys)

	for _, k := range allKeys {
		var vB, vA, vU interface{}
		if rc.Change.Before != nil { vB = rc.Change.Before[k] }
		if rc.Change.After != nil { vA = rc.Change.After[k] }
		if rc.Change.AfterUnknown != nil { vU = rc.Change.AfterUnknown[k] }

		// Indentation: 
		// Resource block at 2. 
		// Attributes start at 6 (2 + 4). 
		// "      + key"
		s.WriteString(stringifyDiff(k, vB, vA, vU, 6))
	}

	// Close Resource Block
	s.WriteString("    }\n") 

	return s.String()
}

func getSymbol(action string) string {
	switch action {
	case "create": return "+"
	case "delete": return "-"
	case "update": return "~"
	case "replace": return "-/+"
	default: return ""
	}
}

// --- 2. TUI STYLING (Lip Gloss) ---

var (
	// Base styles
	baseTabStyle = lipgloss.NewStyle().Padding(0, 1)

	// Tab Colors (Active Background / Inactive Foreground)
	// 0: Create (Green), 1: Destroy (Red), 2: Replace (Purple), 3: Update (Yellow), 4: Import (Blue)
	tabColors = []string{
		"#00AF00", // Green
		"#D70000", // Red
		"#FFAF00", // Yellow
		"#AE00FF", // Purple
		"#00AFFF", // Blue
	}

	// Styles for the list
	itemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(0).Foreground(lipgloss.Color("205")).SetString("> ")
)

func getTabStyle(index int, active bool) lipgloss.Style {
	color := tabColors[index%len(tabColors)]
	
	if active {
		return baseTabStyle.
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color(color))
	}
	
	return baseTabStyle.
		Foreground(lipgloss.Color(color))
}

// --- 3. BUBBLE TEA MODEL ---

type model struct {
	plan       TfPlan
	
	// State
	activeTab  int           // 0: Create, 1: Delete, 2: Update, 3: Import
	cursor     int           // Current position in the list
	viewMode   string        // "list" or "detail"
	
	// Data partitioned by tabs
	lists      map[int][]ResourceChange 
	tabs       []string
	
	// Terminal dimensions
	width      int
	height     int
	viewport   viewport.Model
}

func initialModel(jsonContent string) model {
	var plan TfPlan
	// Use decoder to parse numbers as strings/json.Number to preserve formatting
	dec := json.NewDecoder(strings.NewReader(jsonContent))
	dec.UseNumber()
	err := dec.Decode(&plan)
	if err != nil {
		// handle error if needed, for now just let it be empty plan effectively
	}

	// Partition resources into buckets
	lists := make(map[int][]ResourceChange)
	actionCounter := []int{0, 0, 0, 0, 0}
	for _, rc := range plan.ResourceChanges {
		action := rc.Change.Actions[0]
		
		// Simple mapping logic
		var tabIndex int
		
		// Check for Replace first (delete, create)
		if len(rc.Change.Actions) > 1 && rc.Change.Actions[0] == "delete" && rc.Change.Actions[1] == "create" {
			tabIndex = 2 // Replace
		} else if action == "create" {
			tabIndex = 0
		} else if action == "delete" {
			tabIndex = 1
		} else if action == "update" {
			tabIndex = 3
		} else {
			tabIndex = 4 // Import or other
		}
		lists[tabIndex] = append(lists[tabIndex], rc)
		actionCounter[tabIndex] = actionCounter[tabIndex] + 1
	}
	
	return model{
		plan:      plan,
		activeTab: 0,
		cursor:    0,
		viewMode:  "list",
		lists:     lists,
		tabs:      []string{
			"CREATE (+ " + fmt.Sprintf("%d", actionCounter[0]) + ")", 
			"DESTROY (- " + fmt.Sprintf("%d", actionCounter[1]) + ")", 
			"REPLACE (-/+ " + fmt.Sprintf("%d", actionCounter[2]) + ")",
			"UPDATE (~ " + fmt.Sprintf("%d", actionCounter[3]) + ")", 
			"IMPORT (" + fmt.Sprintf("%d", actionCounter[4]) + ")",
		},
		viewport:  viewport.New(0, 0), // Initial size, will be updated on resize
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

// --- 4. UPDATE (Handle Keyboard Input) ---

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		
		case "ctrl+c", "q":
			return m, tea.Quit

		case "tab", "right":
			if m.viewMode == "list" {
				m.activeTab++
				if m.activeTab >= len(m.tabs) {
					m.activeTab = 0
				}
				m.cursor = 0 // Reset cursor when switching tabs
			}

		case "shift+tab", "left":
			if m.viewMode == "list" {
				m.activeTab--
				if m.activeTab < 0 {
					m.activeTab = len(m.tabs) - 1
				}
				m.cursor = 0
			}

		case "up", "k":
			if m.viewMode == "list" && m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			// Only scroll if we haven't reached the end of the current tab's list
			currentList := m.lists[m.activeTab]
			if m.viewMode == "list" && m.cursor < len(currentList)-1 {
				m.cursor++
			}

		case "enter":
			if m.viewMode == "list" && len(m.lists[m.activeTab]) > 0 {
				m.viewMode = "detail"
				
				// Set viewport content
				selectedRes := m.lists[m.activeTab][m.cursor]
				// renderDiff now includes headers and detailed body
				m.viewport.SetContent(renderDiff(selectedRes))
			}

		case "esc":
			if m.viewMode == "detail" {
				m.viewMode = "list"
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		// Update viewport size (leaving space for header/separator)
		headerHeight := 4 // approx
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight
	}
	
	// Handle viewport updates if active
	if m.viewMode == "detail" {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	
	return m, nil
}

// --- 5. VIEW (Render the UI) ---

func (m model) View() string {
	var s strings.Builder

	// --- A. Render Tabs ---
	var renderedTabs []string
	for i, t := range m.tabs {
		style := getTabStyle(i, m.activeTab == i)
		renderedTabs = append(renderedTabs, style.Render(t))
	}
	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...))
	s.WriteString("\n")
	
	// Render width-aware separator
	sepWidth := m.width
	if sepWidth == 0 {
		sepWidth = 80 // fallback
	}
	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		Render(strings.Repeat("─", sepWidth))

	s.WriteString(separator + "\n\n")

	// --- B. Logic based on View Mode ---
	
	currentList := m.lists[m.activeTab]

	if m.viewMode == "list" {
		// Render List
		if len(currentList) == 0 {
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render("  No changes in this category."))
		} else {
			for i, item := range currentList {
				// Render cursor logic
				if m.cursor == i {
					s.WriteString(selectedItemStyle.Render(item.Address) + "\n")
				} else {
					s.WriteString(itemStyle.Render(item.Address) + "\n")
				}
			}
		}
		s.WriteString("\n\n[Arrows]: Navigate  [Enter]: Details  [Tab]: Next Category  [q]: Quit")
		
	} else {
		// Render Detail View
		s.WriteString(m.viewport.View())
		s.WriteString("\n(Press Esc to go back)")
	}

	return s.String()
}

// --- 6. MAIN ---

func main() {
	lipgloss.SetColorProfile(termenv.TrueColor)
// Paste your JSON string here
	// 1. Check Arguments
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <path-to-tfplan-file>")
	}
	planPath := os.Args[1]

	// 2. Validate file exists
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		log.Fatalf("File does not exist: %s", planPath)
	}

	fmt.Println("🔍 Running 'terraform show -json'...")

	// 3. Run Terraform Command
	// We execute "terraform show -json <planfile>" and capture stdout
	cmd := exec.Command("terraform", "show", "-json", planPath)
	
	// Optional: Set stderr to os.Stderr so you see Terraform errors if it fails
	cmd.Stderr = os.Stderr
	
	output, err := cmd.Output()
	if err != nil {
		log.Fatalf("Failed to run terraform command: %v", err)
	}

	fmt.Println("✅ Terraform JSON generated. Parsing...")

	// 4. Parse JSON
	p := tea.NewProgram(initialModel(string(output)), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
	}
}
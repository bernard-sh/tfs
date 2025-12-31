package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- 1. TYPES & MODELS ---

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

type model struct {
	plan      TfPlan
	activeTab int // 0: Create, 1: Destroy, 2: Replace, 3: Update, 4: Import
	cursor    int
	viewMode  string // "list" or "detail"
	lists     map[int][]ResourceChange
	tabs      []string
	viewport  viewport.Model
}

// --- 2. STYLES ---

var (
	baseTabStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.RoundedBorder(), false, false, false, false)

	activeTabStyle = baseTabStyle.
			Foreground(lipgloss.Color("#282828")).
			Background(lipgloss.Color("#7AA2F7")). // Blue-ish
			Bold(true)

	inactiveTabStyle = baseTabStyle.
			Foreground(lipgloss.Color("#565F89"))

	itemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(lipgloss.Color("#A9B1D6")) // Light Text

	selectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(lipgloss.Color("#7AA2F7")). // Accent
				Bold(true).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color("#7AA2F7"))

	// Tab Colors (Create, Destroy, Replace, Update, Import)
	tabColors = []string{
		"#00AF00", // Green
		"#D70000", // Red
		"#FFAF00", // Orange (Replace)
		"#AE00FF", // Purple (Update)
		"#00AFFF", // Blue (Import)
	}
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

// --- 3. HELPER FUNCTIONS ---

func getSymbol(action string) string {
	switch action {
	case "create":
		return "+"
	case "delete":
		return "-"
	case "update":
		return "~"
	case "replace":
		return "-/+"
	default:
		return ""
	}
}

// Helper to format a value for display
// Moved inside renderDiff in main.go, but here we can make it standalone or method
func formatValue(v interface{}, indent int) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case map[string]interface{}:
		var sb strings.Builder
		sb.WriteString("{\n")
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		padding := strings.Repeat(" ", indent+2)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("%s%s = %s\n", padding, k, formatValue(val[k], indent+2)))
		}
		sb.WriteString(strings.Repeat(" ", indent) + "}")
		return sb.String()
	case []interface{}:
		if len(val) == 0 {
			return "[]"
		}
		var sb strings.Builder
		sb.WriteString("[\n")
		padding := strings.Repeat(" ", indent+2)
		for _, item := range val {
			sb.WriteString(fmt.Sprintf("%s%s,\n", padding, formatValue(item, indent+2)))
		}
		sb.WriteString(strings.Repeat(" ", indent) + "]")
		return sb.String()
	case string:
		return fmt.Sprintf("%q", val)
	case json.Number:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

// Helper to pretty-print attributes with recursive diff style
func renderDiff(rc ResourceChange) string {
	var s strings.Builder

	// Styles
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00AF00")) // Green
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D70000")) // Red
	modStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#AE00FF")) // Purple (Update)
	repStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAF00")) // Orange (Replace)

	// Recursive diff function
	var stringifyDiff func(key string, valBefore, valAfter interface{}, unknown interface{}, indent int, modStyle lipgloss.Style) string
	stringifyDiff = func(key string, valBefore, valAfter interface{}, unknown interface{}, indent int, modStyle lipgloss.Style) string {
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
			for k := range mapBefore {
				seen[k] = true
			}
			for k := range mapAfter {
				seen[k] = true
			}

			allKeys := make([]string, 0, len(seen))
			for k := range seen {
				allKeys = append(allKeys, k)
			}
			sort.Strings(allKeys)

			for _, k := range allKeys {
				var vB, vA interface{}
				if v, ok := mapBefore[k]; ok {
					vB = v
				}
				if v, ok := mapAfter[k]; ok {
					vA = v
				}

				// Recurse (inner lines will be colored themselves)
				sb.WriteString(stringifyDiff(k, vB, vA, nil, indent+4, modStyle))
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

	// Apply color to the resource opening line
	// Also determine modStyle to pass down
	var parentStyle lipgloss.Style

	if action == "create" {
		s.WriteString(addStyle.Render(resourceLine) + "\n")
		parentStyle = addStyle
	} else if action == "delete" {
		s.WriteString(delStyle.Render(resourceLine) + "\n")
		parentStyle = delStyle
	} else if action == "update" {
		s.WriteString(modStyle.Render(resourceLine) + "\n")
		parentStyle = modStyle
	} else if isReplace {
		s.WriteString(repStyle.Render(resourceLine) + "\n")
		parentStyle = repStyle
	} else {
		s.WriteString(resourceLine + "\n")
		parentStyle = lipgloss.NewStyle() // No color
	}

	// Iterate keys
	// Collect top level keys
	seen := make(map[string]bool)
	if rc.Change.Before != nil {
		for k := range rc.Change.Before {
			seen[k] = true
		}
	}
	if rc.Change.After != nil {
		for k := range rc.Change.After {
			seen[k] = true
		}
	}
	for k := range rc.Change.AfterUnknown {
		seen[k] = true
	}

	allKeys := make([]string, 0, len(seen))
	for k := range seen {
		if k == "id" {
			continue
		}
		allKeys = append(allKeys, k)
	}
	sort.Strings(allKeys)

	for _, k := range allKeys {
		var vB, vA, vU interface{}
		if rc.Change.Before != nil {
			vB = rc.Change.Before[k]
		}
		if rc.Change.After != nil {
			vA = rc.Change.After[k]
		}
		if rc.Change.AfterUnknown != nil {
			vU = rc.Change.AfterUnknown[k]
		}

		s.WriteString(stringifyDiff(k, vB, vA, vU, 2, parentStyle))
	}

	s.WriteString("}\n")

	return s.String()
}

// --- 4. MODEL INITIALIZATION ---

func InitialModel(jsonContent string) (tea.Model, error) {
	var plan TfPlan
	// Use decoder to parse numbers as strings/json.Number to preserve formatting
	dec := json.NewDecoder(strings.NewReader(jsonContent))
	dec.UseNumber()
	err := dec.Decode(&plan)
	if err != nil {
		return nil, fmt.Errorf("failed to decode plan JSON: %w", err)
	}

	// Partition resources into buckets
	lists := make(map[int][]ResourceChange)
	actionCounter := []int{0, 0, 0, 0, 0} // CREATE, DESTROY, REPLACE, UPDATE, IMPORT (Fixed order)

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
		tabs: []string{
			"CREATE (+ " + fmt.Sprintf("%d", actionCounter[0]) + ")",
			"DESTROY (- " + fmt.Sprintf("%d", actionCounter[1]) + ")",
			"REPLACE (-/+ " + fmt.Sprintf("%d", actionCounter[2]) + ")",
			"UPDATE (~ " + fmt.Sprintf("%d", actionCounter[3]) + ")",
			"IMPORT (" + fmt.Sprintf("%d", actionCounter[4]) + ")",
		},
		viewport: viewport.New(0, 0), // Initial size, will be updated on resize
	}, nil
}

// --- 5. TEA BOILERPLATE ---

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "tab", "right", "l":
			// Cycle tabs
			m.activeTab++
			if m.activeTab >= len(m.tabs) {
				m.activeTab = 0
			}
			m.cursor = 0        // Reset cursor on tab switch
			m.viewMode = "list" // Reset to list on tab switch
			return m, nil

		case "shift+tab", "left", "h":
			m.activeTab--
			if m.activeTab < 0 {
				m.activeTab = len(m.tabs) - 1
			}
			m.cursor = 0
			m.viewMode = "list"
			return m, nil

		case "up", "k":
			if m.viewMode == "list" {
				if m.cursor > 0 {
					m.cursor--
				}
			} else {
				// Scroll viewport
				m.viewport.LineUp(1)
			}

		case "down", "j":
			if m.viewMode == "list" {
				if m.cursor < len(m.lists[m.activeTab])-1 {
					m.cursor++
				}
			} else {
				m.viewport.LineDown(1)
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
		// Handle resizing
		headerHeight := 3 // Header + Tabs
		footerHeight := 2 // Help text

		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - footerHeight
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	var s strings.Builder

	// --- A. Render Header + Tabs ---

	// Render Tabs
	var tabs []string
	for i, t := range m.tabs {
		style := getTabStyle(i, i == m.activeTab)
		tabs = append(tabs, style.Render(t))
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	s.WriteString(row + "\n")

	// Separator
	// We don't have msg.Width stored readily except in update,
	// might be safer to use a fixed width or store width in model.
	// For now, let's use a standard width or 100 safe.
	sepWidth := m.viewport.Width
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

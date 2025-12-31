package web

import (
	"encoding/json"
	"fmt"
	"os"
)

func GenerateHTML(plan interface{}, outputPath string) error {
	// Plan is passed as interface{} to avoid circular dependency if TfPlan is in TUI
	// But ideally we share models. For now, assume it's valid struct.
    // Wait, main.go defines TfPlan. We should move models to internal/models or similar.
    // Or defining structs here again?
    // Let's create `internal/models/plan.go` ideally.
    // For this step I'll just accept interface{} and marshal it so I don't break things 
    // before seeing where TfPlan goes.
    
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return err
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Terraform Plan Analysis</title>
    <style>
        :root {
            --bg-color: #1a1b26;
            --text-color: #a9b1d6;
            --sidebar-bg: #16161e;
            --border-color: #414868;
            --accent-color: #7aa2f7;
            --create-color: #00AF00;
            --destroy-color: #D70000;
            --update-color: #AE00FF;
            --replace-color: #FFAF00;
            --import-color: #00AFFF;
            --tab-text-inactive: #626262;
            --tab-text-active: #FAFAFA;
        }

        body {
            margin: 0;
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background-color: var(--bg-color);
            color: var(--text-color);
            height: 100vh;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }

        /* HEADER & TABS */
        .header {
            background-color: var(--sidebar-bg);
            border-bottom: 1px solid var(--border-color);
            padding: 0 10px;
            height: 40px;
            display: flex;
            align-items: flex-end;
            user-select: none;
        }

        .tab {
            padding: 8px 16px;
            cursor: pointer;
            font-weight: bold;
            font-size: 14px;
            color: var(--tab-text-inactive);
            border-top-left-radius: 4px;
            border-top-right-radius: 4px;
            margin-right: 2px;
            transition: background 0.2s;
        }

        .tab:hover {
            background-color: rgba(255, 255, 255, 0.05);
        }

        .tab.active {
            color: var(--tab-text-active);
        }
        
        /* Tab Colors */
        .tab-create.active { background-color: var(--create-color); }
        .tab-destroy.active { background-color: var(--destroy-color); }
        .tab-replace.active { background-color: var(--replace-color); }
        .tab-update.active { background-color: var(--update-color); }
        .tab-import.active { background-color: var(--import-color); }
        
        .tab-create { color: var(--create-color); }
        .tab-destroy { color: var(--destroy-color); }
        .tab-replace { color: var(--replace-color); }
        .tab-update { color: var(--update-color); }
        .tab-import { color: var(--import-color); }

        /* MAIN LAYOUT */
        .container {
            display: flex;
            flex: 1;
            overflow: hidden;
        }

        /* SIDEBAR LIST */
        .sidebar {
            width: 350px;
            background-color: var(--sidebar-bg);
            border-right: 1px solid var(--border-color);
            overflow-y: auto;
            display: flex;
            flex-direction: column;
            flex-shrink: 0;
        }

        .resource-item {
            padding: 10px 15px;
            cursor: pointer;
            border-bottom: 1px solid rgba(65, 72, 104, 0.3);
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
            font-size: 14px;
            transition: background 0.1s;
        }

        .resource-item:hover {
            background-color: rgba(255, 255, 255, 0.05);
        }

        .resource-item.selected {
            background-color: rgba(122, 162, 247, 0.15);
            border-left: 3px solid var(--accent-color);
            padding-left: 12px;
        }

        /* DETAIL VIEW */
        .detail-view {
            flex: 1;
            padding: 20px;
            overflow-y: auto;
            font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
            line-height: 1.5;
            font-size: 14px;
        }

        .diff-line { white-space: pre; }
        .diff-add { color: var(--create-color); }
        .diff-del { color: var(--destroy-color); }
        .diff-mod { color: var(--update-color); }
        .diff-rep { color: var(--replace-color); }
        .diff-header { font-weight: bold; margin-bottom: 10px; display: block; }
        .diff-block-header { font-weight: normal; }

        /* SCROLLBAR */
        ::-webkit-scrollbar { width: 10px; height: 10px; }
        ::-webkit-scrollbar-track { background: var(--bg-color); }
        ::-webkit-scrollbar-thumb { background: var(--border-color); border-radius: 5px; }
        ::-webkit-scrollbar-thumb:hover { background: #565f89; }
        
        .empty-state { padding: 40px; text-align: center; color: var(--border-color); }

    </style>
</head>
<body>

<div class="header" id="tabs-container">
    <!-- Tabs will be injected here -->
</div>

<div class="container">
    <div class="sidebar" id="resource-list">
        <!-- Resources will be injected here -->
    </div>
    <div class="detail-view" id="detail-view">
        <div class="empty-state">Select a resource to view details</div>
    </div>
</div>

<script>
    // Embedded Plan Data
    const planData = %s;
    
    // State
    let activeTab = 0;
    let selectedResourceIndex = -1;
    let filteredResources = [];

    // Categories
    const CAT_CREATE = 0;
    const CAT_DESTROY = 1;
    const CAT_REPLACE = 2; // Fixed: Replace is 2 in Go logic now
    const CAT_UPDATE = 3;  // Fixed: Update is 3 in Go logic now
    const CAT_IMPORT = 4;

    function getCategory(rc) {
        const actions = rc.address ? rc.change.actions : rc.Change.Actions; // Handle case sensitivity if raw JSON vs Go struct differs
        // Go struct: rc.Change.Actions. JSON usually: rc.change.actions (lowercase)
        // Let's normalize content
        const act = actions || [];
        
        if (act.length > 1 && act[0] === "delete" && act[1] === "create") return CAT_REPLACE;
        if (act[0] === "create") return CAT_CREATE;
        if (act[0] === "delete") return CAT_DESTROY;
        if (act[0] === "update") return CAT_UPDATE;
        return CAT_IMPORT;
    }

    // Process Data into buckets
    const resourcesByCat = { 0: [], 1: [], 2: [], 3: [], 4: [] };
    
    // Check capitalization from JSON marshal
    // Go "ResourceChanges" -> JSON "resource_changes" usually? 
    // Wait, json.Marshal uses struct tags. Go struct has no tags? 
    // Let's assume standard Go struct field rules or check 'plan.json' viewed earlier.
    // Viewed file 'plan.json': "resource_changes": [ ... "change": { "actions": ... } ]
    // So lowercase underscore.
    
    const allResources = planData.resource_changes || [];

    allResources.forEach(rc => {
        // Skip null changes if any (Terraform sometimes includes no-op resources in plan)
        if (!rc.change || !rc.change.actions || rc.change.actions.length === 0 || rc.change.actions[0] === "no-op") return;
        
        const cat = getCategory(rc);
        resourcesByCat[cat].push(rc);
    });

    function renderTabs() {
        const categories = [
            { id: 0, label: "CREATE", symbol: "+", key: "create" },
            { id: 1, label: "DESTROY", symbol: "-", key: "destroy" },
            { id: 2, label: "REPLACE", symbol: "-/+", key: "replace" },
            { id: 3, label: "UPDATE", symbol: "~", key: "update" },
            { id: 4, label: "IMPORT", symbol: "", key: "import" }
        ];

        const container = document.getElementById('tabs-container');
        container.innerHTML = "";

        categories.forEach(cat => {
            const count = resourcesByCat[cat.id].length;
            const el = document.createElement('div');
            el.className = "tab tab-" + cat.key + (activeTab === cat.id ? " active" : "");
            el.textContent = cat.label + " (" + cat.symbol + " " + count + ")";
            el.onclick = () => switchTab(cat.id);
            container.appendChild(el);
        });
    }

    function switchTab(id) {
        activeTab = id;
        selectedResourceIndex = -1;
        renderTabs();
        renderList();
        renderDetail();
    }

    function renderList() {
        const listContainer = document.getElementById('resource-list');
        listContainer.innerHTML = "";
        
        filteredResources = resourcesByCat[activeTab];

        if (filteredResources.length === 0) {
            const empty = document.createElement('div');
            empty.className = "empty-state";
            empty.textContent = "No resources";
            listContainer.appendChild(empty);
            return;
        }

        filteredResources.forEach((rc, idx) => {
            const el = document.createElement('div');
            el.className = "resource-item" + (selectedResourceIndex === idx ? " selected" : "");
            el.textContent = rc.address;
            el.title = rc.address;
            el.onclick = () => selectResource(idx);
            listContainer.appendChild(el);
        });
    }

    function selectResource(idx) {
        selectedResourceIndex = idx;
        renderList(); // Re-render to update selected class
        renderDetail();
    }

    // --- DIFF RENDERING LOGIC (Ported from Go) ---
    
    function formatValue(v, indent) {
        if (v === null || v === undefined) return "null";
        
        if (typeof v === 'object' && !Array.isArray(v)) {
            // Map
            const keys = Object.keys(v).sort();
            let sb = "{\n";
            const padding = " ".repeat(indent + 4);
            keys.forEach(k => {
                sb += padding + k + " = " + formatValue(v[k], indent + 4) + "\n";
            });
            sb += " ".repeat(indent) + "}";
            return sb;
        } else if (Array.isArray(v)) {
            // List
            if (v.length === 0) return "[]";
            let sb = "[\n";
            const padding = " ".repeat(indent + 4);
            v.forEach(item => {
                sb += padding + formatValue(item, indent + 4) + ",\n";
            });
            sb += " ".repeat(indent) + "]";
            return sb;
        } else if (typeof v === 'string') {
            // Unescape newlines to interpret them as actual line breaks in CSS 'white-space: pre'
            return JSON.stringify(v).replace(/\\n/g, "\n"); 
        } else {
            return String(v);
        }
    }

    function stringifyDiff(key, valBefore, valAfter, unknown, indent, modClass) {
        let sb = "";
        const padding = " ".repeat(indent);
        
        const isUnknown = (unknown === true); 

        // 1. ADDITION
        if (valBefore === null && (valAfter !== null || isUnknown)) {
            let valStr = "(known after apply)";
            if (!isUnknown) valStr = formatValue(valAfter, indent);
            // Additions always Green ("diff-add") regardless of parent action
            return '<div class="diff-line diff-add">' + padding + '+ ' + key + ' = ' + valStr + '</div>';
        }

        // 2. DELETION
        if (valBefore !== null && valAfter === null && !isUnknown) {
            let valStr = formatValue(valBefore, indent);
            // Deletions always Red ("diff-del")
            return '<div class="diff-line diff-del">' + padding + '- ' + key + ' = ' + valStr + '</div>';
        }

        // 3. MODIFICATION
        // Check if map
        const isMapBefore = (valBefore && typeof valBefore === 'object' && !Array.isArray(valBefore));
        const isMapAfter = (valAfter && typeof valAfter === 'object' && !Array.isArray(valAfter));

        if (isMapBefore && isMapAfter) {
            sb += '<div class="diff-line ' + modClass + '">' + padding + '~ ' + key + ' = {</div>';
            
            const seen = new Set([...Object.keys(valBefore), ...Object.keys(valAfter)]);
            const keys = Array.from(seen).sort();
            
            keys.forEach(k => {
                 const vB = valBefore.hasOwnProperty(k) ? valBefore[k] : null;
                 const vA = valAfter.hasOwnProperty(k) ? valAfter[k] : null;
                 sb += stringifyDiff(k, vB, vA, null, indent + 4, modClass);
            });
            
            sb += '<div class="diff-line ' + modClass + '">' + padding + '}</div>';
            return sb;
        }

        // Scalar Update
        let sBefore = formatValue(valBefore, indent);
        let sAfter = "(known after apply)";
        if (!isUnknown) sAfter = formatValue(valAfter, indent);

        if (sBefore !== sAfter) {
             return '<div class="diff-line ' + modClass + '">' + padding + '~ ' + key + ' = ' + sBefore + ' -> ' + sAfter + '</div>';
        }
        
        return ""; // Unchanged
    }

    function renderDetail() {
        const view = document.getElementById('detail-view');
        if (selectedResourceIndex === -1) {
            view.innerHTML = '<div class="empty-state">Select a resource to view details</div>';
            return;
        }

        const rc = filteredResources[selectedResourceIndex];
        const action = rc.change.actions[0];
        const isReplace = (rc.change.actions.length > 1 && rc.change.actions[0] === 'delete');

        // Styles
        let actionClass = "diff-mod"; // Default header style
        let childModClass = "diff-mod"; // Default style for modified attributes
        let symbol = "~";
        let headerText = "# " + rc.type + "." + rc.name + " will be updated in-place";
        
        if (isReplace) {
             actionClass = "diff-rep";
             childModClass = "diff-rep"; // Use replace color for attributes too
             symbol = "-/+";
             headerText = "# " + rc.type + "." + rc.name + " must be replaced";
        } else if (action === "create") {
             actionClass = "diff-add";
             childModClass = "diff-add"; // Creations imply everything new is green
             symbol = "+";
             headerText = "# " + rc.type + "." + rc.name + " will be created";
        } else if (action === "delete") {
             actionClass = "diff-del";
             childModClass = "diff-del";
             symbol = "-";
             headerText = "# " + rc.type + "." + rc.name + " will be destroyed";
        }

        let html = '<div class="diff-header">' + headerText + '</div>';
        
        // Resource Block Open
        html += '<div class="diff-line ' + actionClass + '">  ' + symbol + ' resource "' + rc.type + '" "' + rc.name + '" {</div>';
        
        // Attributes
        const before = rc.change.before || {};
        const after = rc.change.after || {};
        const unknown = rc.change.after_unknown || {};

        const seen = new Set([...Object.keys(before), ...Object.keys(after), ...Object.keys(unknown)]);
        const keys = Array.from(seen).sort().filter(k => k !== 'id');

        keys.forEach(k => {
             const vB = before.hasOwnProperty(k) ? before[k] : null;
             const vA = after.hasOwnProperty(k) ? after[k] : null;
             // Check unknown map. Unmarshal often makes it { key: true }
             const vU = unknown.hasOwnProperty(k) ? unknown[k] : null;
             
             html += stringifyDiff(k, vB, vA, vU, 6, childModClass);
        });

        html += '<div class="diff-line">    }</div>';
        view.innerHTML = html;
    }

    // Init
    renderTabs();
    renderList();
    
</script>
</body>
</html>`, string(planJSON))

	return os.WriteFile(outputPath, []byte(html), 0644)
}

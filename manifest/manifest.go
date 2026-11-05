// Package manifest declares the editorial workflow module (its registry
// entry): states of content items and the transitions between them by
// role, publishing what reaches a published state.
package manifest

import (
	"github.com/kosatka-cms/kosatka-module-workflow/ent"
	"github.com/kosatka-cms/kosatka/modulekit/manifestkit"
	"github.com/kosatka-cms/kosatka/pkg/i18n"
)

// Name - the module's machine name.
const Name = "workflow"

// The module's pages: its settings, the editorial queue.
const (
	SettingsPath = "/admin/config/workflow"
	QueuePath    = "/admin/content/workflow"
)

// PermTakeOver - taking over an item's form locked by another editor.
const PermTakeOver = "take over editing locks"

// Module - the module's registry entry. New to the engine after sites
// were installed: off until enabled.
func Module() manifestkit.Module {
	return manifestkit.Module{
		Name: Name, Label: i18n.N("Editorial workflow"),
		Description: i18n.N("States of content (draft, in review, approved, published, archived - your own) and the transitions between them by role: who sends to review, who approves, who publishes; the editorial queue."),
		Category:    manifestkit.CategoryContent,
		CSS:         []string{"workflow"},
		JS:          []string{"workflow"},
		Tables:      []string{"workflow_states", "workflow_transitions", "workflow_settings", "workflow_node_states", "workflow_history", "workflow_locks"},
		AdminMenu: []manifestkit.ModuleMenuItem{
			{Section: manifestkit.AdminSectionContent, Item: manifestkit.AdminMenuItem{Label: i18n.N("Editorial"), Path: QueuePath}},
			{Section: manifestkit.AdminSectionConfiguration, Item: manifestkit.AdminMenuItem{Label: i18n.N("Editorial workflow"), Path: SettingsPath, Group: manifestkit.AdminGroupSite}},
		},
		Permissions:  []manifestkit.ModulePermission{{Key: PermTakeOver, Label: i18n.N("Take over another's editing lock")}},
		LabelColumns: []manifestkit.ModuleLabelColumn{{Table: "workflow_states", Column: "label"}, {Table: "workflow_transitions", Column: "label"}},
		OffByDefault: true,
	}
}

// RegisterSteps registers the module's own ent schema (its tables and
// migration files) and its install step: the default states and
// transitions.
func RegisterSteps() {
	manifestkit.RegisterSchema(Name, ent.Schema)
	manifestkit.RegisterInstall(Name, install)
}

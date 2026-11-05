package workflow

import (
	"strings"
	"testing"
	"time"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
	"github.com/kosatka-cms/kosatka/modulekit/testkit"
	"github.com/kosatka-cms/kosatka/pkg/pagination"
)

// TestFormCard - the form's card: the state with its colour, a button per
// transition (the first primary), Save keeping the state after a hidden
// default; without transitions, no comment.
func TestFormCard(t *testing.T) {
	testkit.UseEngineRoot(t)
	testkit.WithModules(t, map[string]bool{manifest.Name: true})
	data := map[string]any{"isCreateMode": true, "nodeWorkflow": formData{
		State:       State{Name: "draft", Label: "Draft", Color: "gray"},
		Transitions: []Transition{{ID: 7, Label: "Send to review"}, {ID: 8, Label: "Publish"}},
	}}
	out := testkit.RenderNamed(t, "node_workflow_workflow", data, "partials/workflow_hooks.html")
	for _, want := range []string{
		`<button type="submit" hidden tabindex="-1" aria-hidden="true"></button>`,
		`style="--workflow-color: #8a959c"`, `Draft</span>`,
		`name="workflow_comment"`,
		`<button type="submit" name="workflow_transition" value="7" class="btn btn-primary">Send to review</button>`,
		`value="8" class="btn btn-outline-primary">Publish</button>`, `class="btn btn-outline-secondary">Create</button>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in %s", want, out)
		}
	}
	data["nodeWorkflow"] = formData{State: State{Label: "In review", Color: "amber"}}
	if out := testkit.RenderNamed(t, "node_workflow_workflow", data, "partials/workflow_hooks.html"); strings.Contains(out, "workflow_comment") {
		t.Errorf("a comment without transitions: %s", out)
	}
}

// TestFormLock - the item locked by another: over the form who edits it
// since when, Take over for who may; its card the state and a disabled
// Save only. Locked by the user: the card keeps the lock.
func TestFormLock(t *testing.T) {
	testkit.UseEngineRoot(t)
	testkit.WithModules(t, map[string]bool{manifest.Name: true})
	state := State{Label: "In review", Color: "amber"}
	since := time.Date(2026, 10, 29, 12, 5, 0, 0, time.UTC)
	taken := formData{State: state, Lock: &lockNote{Nid: 5, Who: "anna", Since: since, MayTake: true, Minutes: 30}}
	data := map[string]any{"nodeWorkflow": taken}
	top := testkit.RenderNamed(t, "node_form_top_workflow", data, "partials/workflow_hooks.html")
	for _, want := range []string{`data-workflow-locked`, `anna is editing it - since`, `after 30 minutes without activity`, `action="/workflow/lock/5/take"`} {
		if !strings.Contains(top, want) {
			t.Errorf("no %s in %s", want, top)
		}
	}
	card := testkit.RenderNamed(t, "node_workflow_workflow", data, "partials/workflow_hooks.html")
	if !strings.Contains(card, `In review</span>`) || !strings.Contains(card, " disabled>Save</button>") || strings.Contains(card, "workflow_comment") || strings.Contains(card, " hidden ") {
		t.Errorf("the card of a locked item: %s", card)
	}
	taken.Lock.MayTake = false
	data["nodeWorkflow"] = taken
	if top := testkit.RenderNamed(t, "node_form_top_workflow", data, "partials/workflow_hooks.html"); strings.Contains(top, "/take") {
		t.Errorf("Take over without the permission: %s", top)
	}

	data["nodeWorkflow"] = formData{State: state, Transitions: []Transition{{ID: 3, Label: "Approve"}}, Lock: &lockNote{Nid: 5, Mine: true, Minutes: 15}}
	card = testkit.RenderNamed(t, "node_workflow_workflow", data, "partials/workflow_hooks.html")
	for _, want := range []string{`data-workflow-lock="/workflow/lock/5"`, `data-workflow-lock-minutes="15"`, `data-workflow-lock-lost="Now @name is editing`, `value="3"`} {
		if !strings.Contains(card, want) {
			t.Errorf("no %s in %s", want, card)
		}
	}
	if top := testkit.RenderNamed(t, "node_form_top_workflow", data, "partials/workflow_hooks.html"); strings.Contains(top, "data-workflow-locked") {
		t.Errorf("the user's own lock shown: %s", top)
	}
}

// TestFormNote - over the form, the transition's comment that brought
// the item to its state.
func TestFormNote(t *testing.T) {
	testkit.UseEngineRoot(t)
	testkit.WithModules(t, map[string]bool{manifest.Name: true})
	at := time.Date(2026, 10, 29, 11, 40, 0, 0, time.UTC)
	data := map[string]any{"nodeWorkflow": formData{Note: &transitionNote{Who: "irina", At: at, State: "Draft", Comment: "Add the <address>."}}}
	out := testkit.RenderNamed(t, "node_form_top_workflow", data, "partials/workflow_hooks.html")
	if !strings.Contains(out, "<strong>Draft - irina,") || !strings.Contains(out, "Add the &lt;address&gt;.") {
		t.Errorf("the note: %s", out)
	}
}

// TestQueuePage - the tabs with their counts, a row with its transitions:
// one asking for a comment a link to its page, the other a form back to
// the tab.
func TestQueuePage(t *testing.T) {
	testkit.UseEngineRoot(t)
	testkit.WithModules(t, map[string]bool{manifest.Name: true})
	node := modulekit.Node{ID: 5, Type: "article", Title: "Harbour festival", Slug: "news/harbour"}
	data := map[string]any{"state": "review", "pagination": pagination.New(1, 1, queuePerPage, manifest.QueuePath, nil),
		"tabs": []stateTab{{State: State{Name: "draft", Label: "Draft"}, Count: 3}, {State: State{Name: "review", Label: "In review", Color: "amber"}, Count: 1, Current: true}},
		"rows": []queueRow{{Node: node, TypeLabel: "Article", Author: "petr", Since: NodeState{ChangedAt: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)},
			Transitions: []Transition{{ID: 2, Label: "Return to the author", Comment: "required"}, {ID: 3, Label: "Approve", Comment: "optional"}}, EditedBy: "anna"}}}
	out := testkit.RenderPageContent(t, "admin/workflow_queue.html", data)
	for _, want := range []string{
		`href="/admin/content/workflow?state=review" aria-current="page"`, `In review · 1`, `Draft · 3`,
		`<a href="/news/harbour" class="admin-table-title">Harbour festival</a>`, `<td>Article</td>`, `<td>petr</td>`,
		`<a href="/admin/content/workflow/5/2" class="btn btn-sm btn-outline-secondary">Return to the author</a>`,
		`action="/admin/content/workflow/5/3"`, `name="back" value="/admin/content/workflow?state=review"`,
		`<span class="workflow-edited-by">Being edited by anna</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in %s", want, out)
		}
	}
}

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

// historySample - two transitions, the newest first: the editor
// returning the item to its author, the author sending it to review.
func historySample(open bool) editorialHistory {
	review := State{Name: "review", Label: "In review", Color: "amber"}
	draft := State{Name: "draft", Label: "Draft"}
	return editorialHistory{State: draft, Open: open, Entries: []historyNote{
		{Who: "oleg", Action: "Return to the author", From: review, To: draft, At: time.Date(2026, 11, 11, 16, 5, 0, 0, time.UTC), Comment: "Add the <source>."},
		{Who: "anna", ByAuthor: true, Action: "Send to review", From: draft, To: review, At: time.Date(2026, 11, 11, 15, 20, 0, 0, time.UTC)},
	}}
}

// TestFormHistory - over the form, the item's whole history, the newest
// first: who (the author marked), the button, the states, the comment
// (escaped); open while the item isn't on the site, closed after.
func TestFormHistory(t *testing.T) {
	testkit.UseEngineRoot(t)
	testkit.WithModules(t, map[string]bool{manifest.Name: true})
	h := historySample(true)
	out := testkit.RenderNamed(t, "node_form_top_workflow", map[string]any{"nodeWorkflow": formData{History: &h}}, "partials/workflow_hooks.html")
	for _, want := range []string{
		`<details class="workflow-history-card" open>`, `Editorial history · 2`,
		`<strong>oleg</strong>`, `<span>Return to the author</span>`, `Add the &lt;source&gt;.`,
		`<strong>anna</strong> <span class="workflow-history-role">author</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in %s", want, out)
		}
	}
	if strings.Index(out, "oleg") > strings.Index(out, "anna") {
		t.Errorf("not the newest first: %s", out)
	}
	h = historySample(false)
	if out := testkit.RenderNamed(t, "node_form_top_workflow", map[string]any{"nodeWorkflow": formData{History: &h}}, "partials/workflow_hooks.html"); !strings.Contains(out, `<details class="workflow-history-card">`) {
		t.Errorf("a published item's history open: %s", out)
	}
	if out := testkit.RenderNamed(t, "node_form_top_workflow", map[string]any{"nodeWorkflow": formData{History: &editorialHistory{}}}, "partials/workflow_hooks.html"); strings.Contains(out, "workflow-history") {
		t.Errorf("an empty history shown: %s", out)
	}
}

// TestNodeNotice - over the item on its page (the page's data with the
// module's part): its state, the latest transition, the whole history
// to open, a link to its form; nothing without the module's part.
func TestNodeNotice(t *testing.T) {
	testkit.UseEngineRoot(t)
	testkit.WithModules(t, map[string]bool{manifest.Name: true})
	data := map[string]any{"item": map[string]any{"Node": modulekit.Node{ID: 7}}, "moduleNode": map[string]any{manifest.Name: historySample(true)}}
	out := testkit.RenderNamed(t, "node_notice_workflow", data, "partials/workflow_hooks.html")
	for _, want := range []string{
		`class="workflow-notice"`, `>Draft</span>`, `Not on the site: only its author and editors see it`,
		`<a href="/node/7/edit" class="workflow-notice-edit">Edit</a>`, `Add the &lt;source&gt;.`,
		`<details class="workflow-notice-more">`, `Editorial history · 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in %s", want, out)
		}
	}
	if strings.Count(out, "<strong>oleg</strong>") != 2 || strings.Count(out, "<strong>anna</strong>") != 1 {
		t.Errorf("the latest above, all in the history: %s", out)
	}
	if out := testkit.RenderNamed(t, "node_notice_workflow", map[string]any{"moduleNode": map[string]any{}}, "partials/workflow_hooks.html"); strings.TrimSpace(out) != "" {
		t.Errorf("shown without the module's part: %s", out)
	}
}

// TestQueuePage - the tabs with their counts, a row with its transitions:
// one asking for a comment a link to its page, the other a form back to
// the tab.
func TestQueuePage(t *testing.T) {
	testkit.UseEngineRoot(t)
	testkit.WithModules(t, map[string]bool{manifest.Name: true})
	node := modulekit.Node{ID: 5, Type: "article", Title: "Harbour festival", Slug: "news/harbour"}
	sortURLs := map[string]string{}
	for key := range queueOrder {
		sortURLs[key] = queueSortURL("review", QueueSort{Key: QueueSortUpdated, Desc: true}, key)
	}
	data := map[string]any{"state": "review", "pagination": pagination.New(1, 1, queuePerPage, manifest.QueuePath, nil),
		"sort": QueueSortUpdated, "order": "desc", "sortURLs": sortURLs,
		"tabs": []stateTab{{State: State{Name: "draft", Label: "Draft"}, Count: 3}, {State: State{Name: "review", Label: "In review", Color: "amber"}, Count: 1, Current: true}},
		"rows": []queueRow{{Node: node, TypeLabel: "Article", Author: "petr", Updated: time.Date(2026, 10, 30, 9, 15, 0, 0, time.UTC),
			Transitions: []Transition{{ID: 2, Label: "Return to the author", Comment: "required"}, {ID: 3, Label: "Approve", Comment: "optional"}}, EditedBy: "anna"}}}
	out := testkit.RenderPageContent(t, "admin/workflow_queue.html", data)
	for _, want := range []string{
		`href="/admin/content/workflow?state=review" aria-current="page"`, `In review · 1`, `Draft · 3`,
		`<a href="/news/harbour" class="admin-table-title">Harbour festival</a>`, `<td>Article</td>`, `<td>petr</td>`,
		`<a href="/admin/content/workflow/5/2" class="btn btn-sm btn-outline-secondary">Return to the author</a>`,
		`action="/admin/content/workflow/5/3"`, `name="back" value="/admin/content/workflow?state=review&amp;sort=updated&amp;order=desc"`,
		`<a href="/admin/content/workflow?order=asc&amp;sort=updated&amp;state=review">Updated</a> ↓`,
		`<a href="/admin/content/workflow?order=asc&amp;sort=author&amp;state=review">Author</a>`,
		`<span class="workflow-edited-by">Being edited by anna</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in %s", want, out)
		}
	}
}

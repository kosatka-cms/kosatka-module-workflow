package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
	"github.com/kosatka-cms/kosatka/pkg/i18n"
	"github.com/kosatka-cms/kosatka/pkg/pagination"
)

// queuePerPage - items on a page of the queue.
const queuePerPage = 50

// The queue's sort columns (QueueSort.Key, its "sort" parameter).
const (
	QueueSortUpdated = "updated"
	QueueSortType    = "type"
	QueueSortAuthor  = "author"
	QueueSortTitle   = "title"
)

// QueueSort - the queue's order: a column (QueueSort*) and its direction.
type QueueSort struct {
	Key  string
	Desc bool
}

// queueSortOf - the order r asks for ("sort", "order"): the last saved
// first unless it names a column; a column's own direction
// (queueSortDesc) without "order".
func queueSortOf(r *http.Request) QueueSort {
	q := r.URL.Query()
	key := q.Get("sort")
	if _, ok := queueOrder[key]; !ok {
		key = QueueSortUpdated
	}
	desc := queueSortDesc[key]
	switch q.Get("order") {
	case "asc":
		desc = false
	case "desc":
		desc = true
	}
	return QueueSort{Key: key, Desc: desc}
}

// queueSortDesc - the columns first sorted in descending order (the last
// saved on top).
var queueSortDesc = map[string]bool{QueueSortUpdated: true}

// queueSortURL - the address sorting the queue's state tab by column: the
// other direction when it is sorted by it already.
func queueSortURL(state string, current QueueSort, column string) string {
	desc := queueSortDesc[column]
	if current.Key == column {
		desc = !current.Desc
	}
	order := "asc"
	if desc {
		order = "desc"
	}
	v := url.Values{"state": {state}, "sort": {column}, "order": {order}}
	return manifest.QueuePath + "?" + v.Encode()
}

// palette - the states' colours: a name stored, its hex shown.
var palette = []struct{ Name, Label, Hex string }{
	{"gray", i18n.N("Gray"), "#8a959c"},
	{"amber", i18n.N("Amber"), "#c27c0e"},
	{"blue", i18n.N("Blue"), "#2f6fb2"},
	{"green", i18n.N("Green"), "#1d7a46"},
	{"graphite", i18n.N("Graphite"), "#5d6b74"},
	{"red", i18n.N("Red"), "#b42318"},
	{"purple", i18n.N("Purple"), "#6941c6"},
	{"teal", i18n.N("Teal"), "#0e7490"},
}

// Hex - the state's colour.
func (s State) Hex() string {
	for _, c := range palette {
		if c.Name == s.Color {
			return c.Hex
		}
	}
	return palette[0].Hex
}

// machineName - a state's machine name.
var machineName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Controller - the workflow's settings and the editorial queue.
type Controller struct {
	*modulekit.BaseController
	db    *sql.DB
	w     *Workflow
	nodes modulekit.NodesRepository
	types modulekit.ContentTypeService
	users users
	roles interface {
		Roles(ctx context.Context) (modulekit.Roles, error)
	}
}

// NewController creates a Controller.
func NewController(base *modulekit.BaseController, db *sql.DB, w *Workflow, nodes modulekit.NodesRepository, types modulekit.ContentTypeService, users users, roles interface {
	Roles(ctx context.Context) (modulekit.Roles, error)
}) *Controller {
	base.SetLogger(base.Logger().With("component", "workflow.handler"))
	return &Controller{BaseController: base, db: db, w: w, nodes: nodes, types: types, users: users, roles: roles}
}

// option - a choice of a settings form.
type option struct {
	Value, Label string
	On           bool
}

// Settings renders GET /admin/config/workflow: the states (a blank row
// adds one), the transitions (so), the content types following it.
func (c *Controller) Settings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg, err := c.w.Config(ctx)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "loading the workflow")
		return
	}
	defs, err := c.types.ContentTypes(ctx)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "listing the content types")
		return
	}
	roles, err := c.roleOptions(ctx)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "listing the roles")
		return
	}
	var types []option
	for _, d := range defs {
		types = append(types, option{Value: d.MachineName, Label: d.Label, On: slices.Contains(cfg.Types, d.MachineName)})
	}
	states := append(slices.Clone(cfg.States), State{Color: "gray"})
	transitions := append(slices.Clone(cfg.Transitions), Transition{Comment: manifest.CommentOptional})
	data := c.AdminPageData(r)
	data["title"] = i18n.T(ctx, "Editorial workflow")
	data["pageSEO"] = modulekit.PageSEO{Title: i18n.T(ctx, "@title - Admin panel", "@title", data["title"])}
	data["states"] = states
	data["transitions"] = transitions
	data["types"] = types
	data["roles"] = roles
	data["palette"] = palette
	data["comments"] = []option{{Value: manifest.CommentOptional, Label: i18n.N("optional")}, {Value: manifest.CommentRequired, Label: i18n.N("required")}, {Value: manifest.CommentNone, Label: i18n.N("none")}}
	data["locking"] = cfg.LockMinutes > 0
	data["lockMinutes"] = lockMinuteOptions(cfg.LockMinutes)
	q := r.URL.Query()
	data["saved"] = q.Get("saved") == "1"
	data["error"] = q.Get("error")
	c.RenderOrFail(w, r, "admin/workflow_settings.html", data)
}

// lockChoices - the settings' choices of how long a lock lasts without
// activity, in minutes.
var lockChoices = []int{15, 30, 60, 120}

// minuteOption - a choice of lockChoices.
type minuteOption struct {
	Minutes int
	On      bool
}

// lockMinuteOptions - lockChoices, current (or the default) chosen.
func lockMinuteOptions(current int) []minuteOption {
	if !slices.Contains(lockChoices, current) {
		current = DefaultLockMinutes
	}
	out := make([]minuteOption, len(lockChoices))
	for i, m := range lockChoices {
		out[i] = minuteOption{Minutes: m, On: m == current}
	}
	return out
}

// roleOptions - the site's roles but the guests' (who may make
// transitions).
func (c *Controller) roleOptions(ctx context.Context) ([]option, error) {
	roles, err := c.roles.Roles(ctx)
	if err != nil {
		return nil, err
	}
	var out []option
	for _, r := range roles {
		if r.Name == modulekit.RoleAnonymous {
			continue
		}
		out = append(out, option{Value: r.Name, Label: r.Label()})
	}
	return out, nil
}

// SaveSettings handles POST /admin/config/workflow: the states and
// transitions as the form lists them (indexed fields; a row without a
// label - none, "delete" - dropped), the content types, the locking.
func (c *Controller) SaveSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		c.RenderBadForm(w, r)
		return
	}
	cfg, code := settingsFromForm(r.Form)
	if code != "" {
		http.Redirect(w, r, manifest.SettingsPath+"?error="+code, http.StatusSeeOther)
		return
	}
	defs, err := c.types.ContentTypes(ctx)
	if err != nil {
		c.LogFailure(err, "listing the content types")
		http.Redirect(w, r, manifest.SettingsPath+"?error=save", http.StatusSeeOther)
		return
	}
	for _, d := range defs {
		if slices.Contains(r.Form["types"], d.MachineName) {
			cfg.Types = append(cfg.Types, d.MachineName)
		}
	}
	if err := c.w.SaveConfig(ctx, c.db, cfg); err != nil {
		c.LogFailure(err, "saving the workflow")
		http.Redirect(w, r, manifest.SettingsPath+"?error=save", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, manifest.SettingsPath+"?saved=1", http.StatusSeeOther)
}

// settingsFromForm - the states and transitions form lists, the locking
// (off - LockMinutes 0); a code - what's wrong ("" - nothing).
func settingsFromForm(form url.Values) (Config, string) {
	var cfg Config
	n, _ := strconv.Atoi(form.Get("states_count"))
	initial := form.Get("state_initial")
	for i := range n {
		key := strconv.Itoa(i)
		label := strings.TrimSpace(form.Get("state_label_" + key))
		name := strings.TrimSpace(form.Get("state_name_" + key))
		if form.Get("state_delete_"+key) == "1" || (label == "" && name == "") {
			continue
		}
		if label == "" || !machineName.MatchString(name) {
			return cfg, "state"
		}
		if _, dup := cfg.State(name); dup {
			return cfg, "state-duplicate"
		}
		color := form.Get("state_color_" + key)
		if !slices.ContainsFunc(palette, func(p struct{ Name, Label, Hex string }) bool { return p.Name == color }) {
			color = palette[0].Name
		}
		cfg.States = append(cfg.States, State{Name: name, Label: label, Color: color, Published: form.Get("state_published_"+key) == "1",
			Initial: initial == key, Weight: len(cfg.States) * 10})
	}
	if len(cfg.States) == 0 {
		return cfg, "states"
	}
	if !slices.ContainsFunc(cfg.States, func(s State) bool { return s.Initial }) {
		cfg.States[0].Initial = true
	}
	n, _ = strconv.Atoi(form.Get("transitions_count"))
	for i := range n {
		key := strconv.Itoa(i)
		label := strings.TrimSpace(form.Get("t_label_" + key))
		if form.Get("t_delete_"+key) == "1" || label == "" {
			continue
		}
		to := form.Get("t_to_" + key)
		if _, ok := cfg.State(to); !ok {
			return cfg, "transition"
		}
		var from []string
		for _, f := range form["t_from_"+key] {
			if _, ok := cfg.State(f); ok && f != to {
				from = append(from, f)
			}
		}
		if len(from) == 0 {
			return cfg, "transition"
		}
		comment := form.Get("t_comment_" + key)
		if !slices.Contains([]string{manifest.CommentOptional, manifest.CommentRequired, manifest.CommentNone}, comment) {
			comment = manifest.CommentOptional
		}
		cfg.Transitions = append(cfg.Transitions, Transition{From: from, To: to, Label: label, Roles: form["t_roles_"+key], Comment: comment, Weight: len(cfg.Transitions) * 10})
	}
	if form.Get("locking") == "1" {
		cfg.LockMinutes = DefaultLockMinutes
		if m, _ := strconv.Atoi(form.Get("lock_minutes")); slices.Contains(lockChoices, m) {
			cfg.LockMinutes = m
		}
	}
	return cfg, ""
}

// queueRow - an item of the queue: its node, author, since when in its
// state, the transitions the user may make.
type queueRow struct {
	Node        modulekit.Node
	TypeLabel   string
	Author      string
	Updated     time.Time
	Transitions []Transition
	// EditedBy - who has its form open now ("" - nobody).
	EditedBy string
}

// stateTab - a state of the queue's tabs: its items' count.
type stateTab struct {
	State
	Count   int
	Current bool
}

// Queue renders GET /admin/content/workflow?state=…: the items in a
// state (the first waiting one - not initial, not published - by
// default), the longest waiting first, with the transitions the user may
// make from it.
func (c *Controller) Queue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg, err := c.w.Config(ctx)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "loading the workflow")
		return
	}
	counts, err := c.w.repo.Counts(ctx)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "counting the items by state")
		return
	}
	current := r.URL.Query().Get("state")
	if _, ok := cfg.State(current); !ok {
		current = defaultQueueState(cfg)
	}
	var tabs []stateTab
	for _, s := range cfg.States {
		tabs = append(tabs, stateTab{State: s, Count: counts[s.Name], Current: s.Name == current})
	}
	user, _ := c.GetCurrentUser(r)
	allowed, err := c.w.Allowed(ctx, user, current)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "loading the transitions")
		return
	}
	page := c.GetPageNumber(r)
	sort := queueSortOf(r)
	items, total, err := c.w.repo.InState(ctx, current, sort, queuePerPage, (page-1)*queuePerPage)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "listing the queue")
		return
	}
	labels := map[string]string{}
	if defs, err := c.types.ContentTypes(ctx); err == nil {
		for _, d := range defs {
			labels[d.MachineName] = d.Label
		}
	}
	rows, err := c.queueRows(ctx, items, labels, allowed)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "loading the queue's content")
		return
	}
	data := c.AdminPageData(r)
	data["title"] = i18n.T(ctx, "Editorial")
	data["pageSEO"] = modulekit.PageSEO{Title: i18n.T(ctx, "@title - Admin panel", "@title", data["title"])}
	data["tabs"] = tabs
	data["state"] = current
	// sort, order and the columns' addresses: the headers sort the tab.
	data["sort"] = sort.Key
	data["order"] = map[bool]string{false: "asc", true: "desc"}[sort.Desc]
	sortURLs := map[string]string{}
	for key := range queueOrder {
		sortURLs[key] = queueSortURL(current, sort, key)
	}
	data["sortURLs"] = sortURLs
	data["rows"] = rows
	data["total"] = total
	q := r.URL.Query()
	data["done"] = q.Get("done") == "1"
	data["error"] = q.Get("error")
	data["pagination"] = pagination.New(page, total, queuePerPage, manifest.QueuePath, q)
	c.RenderOrFail(w, r, "admin/workflow_queue.html", data)
}

// queueRows - the queue's items as rows: their content, authors and who
// is editing them, read for the whole page at once (a missing item - one
// deleted meanwhile - is left out).
func (c *Controller) queueRows(ctx context.Context, items []QueueItem, labels map[string]string, allowed []Transition) ([]queueRow, error) {
	ids := make([]int, len(items))
	for i, it := range items {
		ids[i] = it.Nid
	}
	nodes, err := c.nodes.ByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	locks := c.activeLocks(ctx, items)
	var userIDs []int
	for _, n := range nodes {
		userIDs = append(userIDs, n.CreatedBy.ID)
	}
	for _, l := range locks {
		userIDs = append(userIDs, l.UID)
	}
	names := map[int]string{}
	if c.users != nil {
		people, err := c.users.ByIDs(ctx, userIDs)
		if err != nil {
			return nil, err
		}
		for id, u := range people {
			names[id] = u.Username
		}
	}
	var rows []queueRow
	for _, it := range items {
		node, ok := nodes[it.Nid]
		if !ok {
			continue
		}
		row := queueRow{Node: node, TypeLabel: labels[string(node.Type)], Author: names[node.CreatedBy.ID], Updated: it.Updated, Transitions: allowed}
		if l, ok := locks[it.Nid]; ok {
			row.EditedBy = names[l.UID]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// activeLocks - the active locks of items, by id (none - items aren't
// locked, or they can't be read: the queue goes without them).
func (c *Controller) activeLocks(ctx context.Context, items []QueueItem) map[int]Lock {
	since, _, on, err := c.w.lockSince(ctx)
	if err != nil || !on {
		return nil
	}
	ids := make([]int, len(items))
	for i, it := range items {
		ids[i] = it.Nid
	}
	locks, err := c.w.repo.Locks(ctx, ids, since)
	if err != nil {
		modulekit.LogFallback(c.Logger(), err, "loading the queue's locks (none shown)")
		return nil
	}
	return locks
}

// defaultQueueState - the queue's first tab: the first state neither
// initial nor published, else the first.
func defaultQueueState(cfg Config) string {
	for _, s := range cfg.States {
		if !s.Initial && !s.Published {
			return s.Name
		}
	}
	if len(cfg.States) > 0 {
		return cfg.States[0].Name
	}
	return ""
}

// transitionTarget - the item and the transition of a queue action, if
// the user may make it from the item's state: an item of a type the
// workflow handles, one the user may edit.
func (c *Controller) transitionTarget(w http.ResponseWriter, r *http.Request) (*modulekit.Node, State, *Transition, *modulekit.User, bool) {
	ctx := r.Context()
	nid, err1 := strconv.Atoi(r.PathValue("nid"))
	tid, err2 := strconv.Atoi(r.PathValue("tid"))
	if err1 != nil || err2 != nil {
		c.RenderNotFound(w, r)
		return nil, State{}, nil, nil, false
	}
	node, err := c.nodes.ByID(ctx, nid)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "loading the item", "nodeID", nid)
		return nil, State{}, nil, nil, false
	}
	// A transition's permission isn't one over every content: not an
	// item of another type (StateOf would give it a state), nor one the
	// user may not edit.
	user, _ := c.GetCurrentUser(r)
	if !c.w.Handles(ctx, string(node.Type)) || !c.AuthService().CanEditNode(ctx, node, user) {
		c.RenderForbidden(w, r)
		return nil, State{}, nil, nil, false
	}
	state, err := c.w.StateOf(ctx, *node)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "loading the item's state", "nodeID", nid)
		return nil, State{}, nil, nil, false
	}
	allowed, err := c.w.Allowed(ctx, user, state.Name)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "loading the transitions")
		return nil, State{}, nil, nil, false
	}
	i := slices.IndexFunc(allowed, func(t Transition) bool { return t.ID == tid })
	if i < 0 {
		http.Redirect(w, r, manifest.QueuePath+"?state="+url.QueryEscape(state.Name)+"&error=transition", http.StatusSeeOther)
		return nil, State{}, nil, nil, false
	}
	return node, state, &allowed[i], user, true
}

// TransitionPage renders GET /admin/content/workflow/{nid}/{tid}: the
// transition's confirmation with its comment.
func (c *Controller) TransitionPage(w http.ResponseWriter, r *http.Request) {
	node, state, t, _, ok := c.transitionTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	data := c.AdminPageData(r)
	data["title"] = t.Label
	data["pageSEO"] = modulekit.PageSEO{Title: i18n.T(ctx, "@title - Admin panel", "@title", t.Label)}
	data["node"] = node
	data["from"] = state
	data["transition"] = t
	data["error"] = r.URL.Query().Get("error")
	c.RenderOrFail(w, r, "admin/workflow_transition.html", data)
}

// Transition handles POST /admin/content/workflow/{nid}/{tid}: the item
// moved by the transition (with its comment, when it takes one), its
// publishing changed with the item's history; back to the queue.
func (c *Controller) Transition(w http.ResponseWriter, r *http.Request) {
	node, state, t, user, ok := c.transitionTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	comment := strings.TrimSpace(r.FormValue("comment"))
	if t.Comment == manifest.CommentNone {
		comment = ""
	}
	page := fmt.Sprintf("%s/%d/%d", manifest.QueuePath, node.ID, t.ID)
	if t.Comment == manifest.CommentRequired && comment == "" {
		http.Redirect(w, r, page+"?error=comment", http.StatusSeeOther)
		return
	}
	err := modulekit.ChangeNode(ctx, node.ID, user, func(ctx context.Context) error {
		return c.w.Make(ctx, *node, state, *t, user, comment)
	})
	if err != nil {
		c.LogFailure(err, "making a transition", "nodeID", node.ID, "transition", t.ID)
		http.Redirect(w, r, manifest.QueuePath+"?state="+url.QueryEscape(state.Name)+"&error=save", http.StatusSeeOther)
		return
	}
	back := manifest.QueuePath + "?state=" + url.QueryEscape(state.Name) + "&done=1"
	if b := modulekit.SafeRedirectPath(r.FormValue("back")); b != "" {
		back = b
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// waiting - the items the user may move on: in a state neither initial
// nor published from which one of the transitions is theirs (the
// Editorial menu item's count).
func (c *Controller) waiting(ctx context.Context) (int, error) {
	uid, ok := modulekit.ContextUserID(ctx)
	if !ok {
		return 0, nil
	}
	user, err := c.users.ByID(ctx, uid)
	if err != nil {
		return 0, err
	}
	cfg, err := c.w.Config(ctx)
	if err != nil {
		return 0, err
	}
	counts, err := c.w.repo.Counts(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, s := range cfg.States {
		if s.Initial || s.Published || counts[s.Name] == 0 {
			continue
		}
		if allowed, err := c.w.Allowed(ctx, user, s.Name); err == nil && len(allowed) > 0 {
			total += counts[s.Name]
		}
	}
	return total, nil
}

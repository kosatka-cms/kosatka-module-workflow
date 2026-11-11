package workflow

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
)

// historyNote - a transition made, as an item's history shows it: who
// made it (ByAuthor - the item's author), its button (Action, "" when no
// transition of the settings goes so now), the states it went from and
// to, when, with what comment.
type historyNote struct {
	Who      string
	ByAuthor bool
	Action   string
	From, To State
	At       time.Time
	Comment  string
}

// editorialHistory - an item's editorial history as shown: its state,
// every transition made, the newest first; Open - shown expanded (the
// item isn't on the site).
type editorialHistory struct {
	State   State
	Entries []historyNote
	Open    bool
}

// historyOf - node's history in state, for showing: the names of who
// made each transition (users), the transitions' buttons and the states'
// labels and colours as the settings give them now.
func (w *Workflow) historyOf(ctx context.Context, node modulekit.Node, state State, users users) (editorialHistory, error) {
	h := editorialHistory{State: state, Open: !state.Published}
	cfg, err := w.Config(ctx)
	if err != nil {
		return h, err
	}
	entries, err := w.repo.History(ctx, node.ID, 0)
	if err != nil {
		return h, err
	}
	names := map[int]string{}
	if users != nil && len(entries) > 0 {
		var ids []int
		for _, e := range entries {
			if e.UID > 0 && !slices.Contains(ids, e.UID) {
				ids = append(ids, e.UID)
			}
		}
		people, err := users.ByIDs(ctx, ids)
		if err != nil {
			return h, err
		}
		for id, u := range people {
			names[id] = u.Username
		}
	}
	for _, e := range entries {
		h.Entries = append(h.Entries, historyNote{
			Who:      names[e.UID],
			ByAuthor: e.UID > 0 && e.UID == node.CreatedBy.ID,
			Action:   cfg.transitionLabel(e.From, e.To),
			From:     cfg.stateOrName(e.From),
			To:       cfg.stateOrName(e.To),
			At:       e.At,
			Comment:  e.Comment,
		})
	}
	return h, nil
}

// transitionLabel - the button of the transition going from from to to
// ("" - none such now).
func (c Config) transitionLabel(from, to string) string {
	for _, t := range c.Transitions {
		if t.To == to && slices.Contains(t.From, from) {
			return t.Label
		}
	}
	return ""
}

// stateOrName - the state named name; one deleted since - its name as
// its label.
func (c Config) stateOrName(name string) State {
	if s, ok := c.State(name); ok {
		return s
	}
	return State{Name: name, Label: name}
}

// nodePage - the workflow's part of an item's page (modulekit.AddNodePage):
// for who may edit an item of a type following the workflow while it
// isn't on the site, its state and history - the node_notice template;
// for everyone else, and once it is on the site, nothing.
type nodePage struct {
	w      *Workflow
	users  users
	auth   modulekit.AuthService
	logger *slog.Logger
	// viewer - the signed-in user's id from ctx (nil -
	// modulekit.ContextUserID).
	viewer func(ctx context.Context) (int, bool)
}

// parts - see nodePage.
func (p nodePage) parts(ctx context.Context, node modulekit.Node) modulekit.NodePageParts {
	if node.IsPublic() || p.auth == nil || p.users == nil || !p.w.Handles(ctx, string(node.Type)) {
		return modulekit.NodePageParts{}
	}
	viewer := p.viewer
	if viewer == nil {
		viewer = modulekit.ContextUserID
	}
	uid, ok := viewer(ctx)
	if !ok || uid <= 0 {
		return modulekit.NodePageParts{}
	}
	user, err := p.users.ByID(ctx, uid)
	if err != nil || user == nil {
		modulekit.LogFallback(p.logger, err, "loading the viewer of an item's editorial history (not shown)", "nodeID", node.ID)
		return modulekit.NodePageParts{}
	}
	if !p.auth.CanEditNode(ctx, &node, user) {
		return modulekit.NodePageParts{}
	}
	state, err := p.w.StateOf(ctx, node)
	if err == nil {
		var h editorialHistory
		if h, err = p.w.historyOf(ctx, node, state, p.users); err == nil {
			return modulekit.NodePageParts{Data: h}
		}
	}
	modulekit.LogFallback(p.logger, err, "loading an item's editorial history (not shown)", "nodeID", node.ID, "module", manifest.Name)
	return modulekit.NodePageParts{}
}

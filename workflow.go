// Package workflow - the editorial workflow module: content items of the
// types following it move between states (draft, in review, …) by
// transitions the roles may make; a state publishes or unpublishes the
// item.
package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
)

// State - a state of an item (workflow_states).
type State struct {
	ID        int
	Name      string
	Label     string
	Color     string
	Published bool
	Initial   bool
	Weight    int
}

// Transition - a move between states (workflow_transitions): from one of
// From to To, by the button Label, for Roles; Comment - manifest's
// Comment*.
type Transition struct {
	ID      int
	From    []string
	To      string
	Label   string
	Roles   []string
	Comment string
	Weight  int
}

// Config - the workflow; LockMinutes - how long an item's form stays
// locked without its editor's activity (0 - items aren't locked).
type Config struct {
	States      []State
	Transitions []Transition
	Types       []string
	LockMinutes int
}

// DefaultLockMinutes - LockMinutes until the settings are saved.
const DefaultLockMinutes = 30

// State - the state named name (false - none such).
func (c Config) State(name string) (State, bool) {
	i := slices.IndexFunc(c.States, func(s State) bool { return s.Name == name })
	if i < 0 {
		return State{}, false
	}
	return c.States[i], true
}

// Initial - a new item's state.
func (c Config) Initial() State {
	for _, s := range c.States {
		if s.Initial {
			return s
		}
	}
	if len(c.States) > 0 {
		return c.States[0]
	}
	return State{}
}

// Waiting - the first state neither initial nor published (in review,
// say), else the initial one: an item on moderation when its type joins.
func (c Config) Waiting() State {
	for _, s := range c.States {
		if !s.Initial && !s.Published {
			return s
		}
	}
	return c.Initial()
}

// FirstPublished - the first published state ("" - none).
func (c Config) FirstPublished() string {
	for _, s := range c.States {
		if s.Published {
			return s.Name
		}
	}
	return ""
}

// NodeState - an item's recorded state.
type NodeState struct {
	Nid       int
	State     string
	ChangedAt time.Time
	ChangedBy int
}

// HistoryEntry - a transition made.
type HistoryEntry struct {
	Nid     int
	From    string
	To      string
	UID     int
	Comment string
	At      time.Time
}

// Workflow - the workflow's settings (read once, kept until saved) and
// moving items between its states.
type Workflow struct {
	repo  *Repo
	nodes modulekit.NodesRepository
	auth  modulekit.AuthService
	now   func() time.Time
	// mail - a transition's e-mails (nil - none: tests).
	mail *mailer

	mu  sync.Mutex
	cfg *Config
}

// NewWorkflow creates a Workflow.
func NewWorkflow(repo *Repo, nodes modulekit.NodesRepository, auth modulekit.AuthService) *Workflow {
	return &Workflow{repo: repo, nodes: nodes, auth: auth, now: time.Now}
}

// Config - the workflow's settings.
func (w *Workflow) Config(ctx context.Context) (Config, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cfg == nil {
		cfg, err := w.repo.Config(ctx)
		if err != nil {
			return Config{}, err
		}
		w.cfg = &cfg
	}
	return *w.cfg, nil
}

// SaveConfig saves the workflow's settings; the items of a type newly
// following it get a state (Repo.Backfill): those on moderation the
// waiting one, unpublished and off moderation as it is - a change of the
// site's, not an edit: their last save stays (SetPublishing).
func (w *Workflow) SaveConfig(ctx context.Context, db *sql.DB, cfg Config) error {
	err := modulekit.InTx(ctx, db, func(ctx context.Context) error {
		if err := w.repo.SaveConfig(ctx, cfg); err != nil {
			return err
		}
		waiting := cfg.Waiting()
		moderated, err := w.repo.Backfill(ctx, cfg.Types, joinStates{Published: cfg.FirstPublished(), Initial: cfg.Initial().Name, Waiting: waiting.Name}, w.now())
		if err != nil {
			return err
		}
		status := 0
		if waiting.Published {
			status = 1
		}
		for _, nid := range moderated {
			if err := w.nodes.SetPublishing(ctx, nid, status, 0); err != nil {
				return fmt.Errorf("the publishing of node %d joining the workflow: %w", nid, err)
			}
		}
		return nil
	})
	// Read again: the rows got new ids.
	w.mu.Lock()
	w.cfg = nil
	w.mu.Unlock()
	return err
}

// Handles - contentType's items follow the workflow.
func (w *Workflow) Handles(ctx context.Context, contentType string) bool {
	if !modulekit.ModuleEnabled(manifest.Name) {
		return false
	}
	cfg, err := w.Config(ctx)
	return err == nil && len(cfg.States) > 0 && slices.Contains(cfg.Types, contentType)
}

// StateOf - node's state: its recorded one, else what its publishing says
// (on moderation - the waiting state, published - the first published
// one, else the initial one).
func (w *Workflow) StateOf(ctx context.Context, node modulekit.Node) (State, error) {
	cfg, err := w.Config(ctx)
	if err != nil {
		return State{}, err
	}
	if node.ID > 0 {
		ns, ok, err := w.repo.NodeState(ctx, node.ID)
		if err != nil {
			return State{}, err
		}
		if s, known := cfg.State(ns.State); ok && known {
			return s, nil
		}
	}
	if node.OnModeration == 1 {
		return cfg.Waiting(), nil
	}
	if node.Status == 1 {
		if s, ok := cfg.State(cfg.FirstPublished()); ok {
			return s, nil
		}
	}
	return cfg.Initial(), nil
}

// Allowed - the transitions from state user may make: one of its roles
// is the transition's, or the user has full access.
func (w *Workflow) Allowed(ctx context.Context, user *modulekit.User, state string) ([]Transition, error) {
	cfg, err := w.Config(ctx)
	if err != nil || user == nil {
		return nil, err
	}
	full := w.auth != nil && w.auth.HasPermission(ctx, user, modulekit.PermFullAccess)
	return cfg.allowedFrom(state, modulekit.UserRoleNames(user), full), nil
}

// allowedFrom - the transitions from state of a user with roles: those
// naming one of them, every one with full access (full).
func (c Config) allowedFrom(state string, roles []string, full bool) []Transition {
	var out []Transition
	for _, t := range c.Transitions {
		if !slices.Contains(t.From, state) {
			continue
		}
		if full || slices.ContainsFunc(t.Roles, func(r string) bool { return slices.Contains(roles, r) }) {
			out = append(out, t)
		}
	}
	return out
}

// Make moves node from its state by t (by user, with comment): records
// it, sets the node's publishing by the new state (in ctx's
// transaction: the caller tracks the change), then sends its e-mails in
// the background (mailer.later).
func (w *Workflow) Make(ctx context.Context, node modulekit.Node, from State, t Transition, user *modulekit.User, comment string) error {
	cfg, err := w.Config(ctx)
	if err != nil {
		return err
	}
	to, ok := cfg.State(t.To)
	if !ok {
		return fmt.Errorf("the transition's state %q is gone", t.To)
	}
	uid := 0
	if user != nil {
		uid = user.ID
	}
	now := w.now()
	if err := w.repo.SetNodeState(ctx, node.ID, to.Name, uid, now); err != nil {
		return err
	}
	if err := w.repo.AddHistory(ctx, HistoryEntry{Nid: node.ID, From: from.Name, To: to.Name, UID: uid, Comment: comment, At: now}); err != nil {
		return err
	}
	if err := w.publish(ctx, node.ID, to); err != nil {
		return err
	}
	w.mail.later(cfg, transition{Node: node, By: uid, Action: t.Label, To: to, Comment: comment})
	return nil
}

// publish sets node nid's publishing by state: published or not, never
// on moderation.
func (w *Workflow) publish(ctx context.Context, nid int, state State) error {
	status := 0
	if state.Published {
		status = 1
	}
	if err := w.nodes.UpdateStatus(ctx, nid, status); err != nil {
		return fmt.Errorf("publishing node %d: %w", nid, err)
	}
	if err := w.nodes.UpdateOnModeration(ctx, nid, 0); err != nil {
		return fmt.Errorf("publishing node %d: %w", nid, err)
	}
	return nil
}

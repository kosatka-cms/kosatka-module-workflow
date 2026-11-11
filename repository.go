package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/kosatka-cms/kosatka-module-workflow/ent/generated"
	"github.com/kosatka-cms/kosatka-module-workflow/ent/generated/history"
	"github.com/kosatka-cms/kosatka-module-workflow/ent/generated/lock"
	"github.com/kosatka-cms/kosatka-module-workflow/ent/generated/nodestate"
	"github.com/kosatka-cms/kosatka-module-workflow/ent/generated/workflowstate"
	"github.com/kosatka-cms/kosatka-module-workflow/ent/generated/workflowtransition"
	"github.com/kosatka-cms/kosatka/modulekit"
)

// Repo - the module's tables (Backfill reads the core's nodes).
// A built-in module, tested with the engine, may read the core's tables
// in its SQL; a module of its own repository goes through the SDK's
// services instead (docs/modules/sdk.md): the core's columns aren't its
// API.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a Repo on the site's database.
func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// client - the module's client in ctx's transaction, else on the database.
func (r *Repo) client(ctx context.Context) *generated.Client {
	return generated.NewClient(generated.Driver(modulekit.Driver(ctx, r.db)))
}

// Config - the workflow: its states and transitions in order, the
// content types following it.
func (r *Repo) Config(ctx context.Context) (Config, error) {
	c := r.client(ctx)
	states, err := c.WorkflowState.Query().Order(workflowstate.ByWeight(), workflowstate.ByID()).All(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("loading the workflow's states: %w", err)
	}
	transitions, err := c.WorkflowTransition.Query().Order(workflowtransition.ByWeight(), workflowtransition.ByID()).All(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("loading the workflow's transitions: %w", err)
	}
	var cfg Config
	for _, s := range states {
		cfg.States = append(cfg.States, State{ID: s.ID, Name: s.MachineName, Label: s.Label, Color: s.Color, Published: s.Published, Initial: s.Initial, Weight: s.Weight})
	}
	for _, t := range transitions {
		cfg.Transitions = append(cfg.Transitions, Transition{ID: t.ID, From: t.FromStates, To: t.ToState, Label: t.Label, Roles: t.Roles, Comment: t.Comment, Weight: t.Weight})
	}
	settings, err := c.WorkflowSettings.Get(ctx, 1)
	switch {
	case err == nil:
		cfg.Types = settings.ContentTypes
		cfg.LockMinutes = settings.LockMinutes
	case generated.IsNotFound(err):
		cfg.LockMinutes = DefaultLockMinutes
	default:
		return Config{}, fmt.Errorf("loading the workflow's settings: %w", err)
	}
	return cfg, nil
}

// SaveConfig replaces the workflow's states, transitions and content
// types with cfg's (in ctx's transaction).
func (r *Repo) SaveConfig(ctx context.Context, cfg Config) error {
	c := r.client(ctx)
	if _, err := c.WorkflowState.Delete().Exec(ctx); err != nil {
		return fmt.Errorf("saving the workflow's states: %w", err)
	}
	for _, s := range cfg.States {
		if err := c.WorkflowState.Create().SetMachineName(s.Name).SetLabel(s.Label).SetColor(s.Color).SetPublished(s.Published).
			SetInitial(s.Initial).SetWeight(s.Weight).Exec(ctx); err != nil {
			return fmt.Errorf("saving the workflow's state %s: %w", s.Name, err)
		}
	}
	if _, err := c.WorkflowTransition.Delete().Exec(ctx); err != nil {
		return fmt.Errorf("saving the workflow's transitions: %w", err)
	}
	for _, t := range cfg.Transitions {
		roles := t.Roles
		if roles == nil {
			roles = []string{}
		}
		if err := c.WorkflowTransition.Create().SetFromStates(t.From).SetToState(t.To).SetLabel(t.Label).SetRoles(roles).
			SetComment(t.Comment).SetWeight(t.Weight).Exec(ctx); err != nil {
			return fmt.Errorf("saving the workflow's transition to %s: %w", t.To, err)
		}
	}
	types := cfg.Types
	if types == nil {
		types = []string{}
	}
	err := c.WorkflowSettings.UpdateOneID(1).SetContentTypes(types).SetLockMinutes(cfg.LockMinutes).Exec(ctx)
	if generated.IsNotFound(err) {
		err = c.WorkflowSettings.Create().SetID(1).SetContentTypes(types).SetLockMinutes(cfg.LockMinutes).Exec(ctx)
	}
	if err != nil {
		return fmt.Errorf("saving the workflow's settings: %w", err)
	}
	return nil
}

// NodeState - the state recorded for nid (false - none yet).
func (r *Repo) NodeState(ctx context.Context, nid int) (NodeState, bool, error) {
	row, err := r.client(ctx).NodeState.Query().Where(nodestate.Nid(nid)).Only(ctx)
	if generated.IsNotFound(err) {
		return NodeState{}, false, nil
	}
	if err != nil {
		return NodeState{}, false, fmt.Errorf("loading the state of node %d: %w", nid, err)
	}
	return NodeState{Nid: row.Nid, State: row.State, ChangedAt: row.ChangedAt, ChangedBy: row.ChangedBy}, true, nil
}

// SetNodeState records nid's state, since now, by uid.
func (r *Repo) SetNodeState(ctx context.Context, nid int, state string, uid int, now time.Time) error {
	c := r.client(ctx)
	n, err := c.NodeState.Update().Where(nodestate.Nid(nid)).SetState(state).SetChangedAt(now).SetChangedBy(uid).Save(ctx)
	if err == nil && n == 0 {
		err = c.NodeState.Create().SetNid(nid).SetState(state).SetChangedAt(now).SetChangedBy(uid).Exec(ctx)
	}
	if err != nil {
		return fmt.Errorf("setting the state of node %d: %w", nid, err)
	}
	return nil
}

// AddHistory records a transition of nid.
func (r *Repo) AddHistory(ctx context.Context, h HistoryEntry) error {
	if err := r.client(ctx).History.Create().SetNid(h.Nid).SetFromState(h.From).SetToState(h.To).SetUID(h.UID).
		SetComment(h.Comment).SetCreatedAt(h.At).Exec(ctx); err != nil {
		return fmt.Errorf("recording a transition of node %d: %w", h.Nid, err)
	}
	return nil
}

// History - nid's transitions, the newest first (limit 0 - all).
func (r *Repo) History(ctx context.Context, nid, limit int) ([]HistoryEntry, error) {
	q := r.client(ctx).History.Query().Where(history.Nid(nid)).Order(history.ByCreatedAt(entsql.OrderDesc()), history.ByID(entsql.OrderDesc()))
	if limit > 0 {
		q = q.Limit(limit)
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading the transitions of node %d: %w", nid, err)
	}
	out := make([]HistoryEntry, len(rows))
	for i, h := range rows {
		out[i] = HistoryEntry{Nid: h.Nid, From: h.FromState, To: h.ToState, UID: h.UID, Comment: h.Comment, At: h.CreatedAt}
	}
	return out, nil
}

// QueueItem - an item of the queue: its state and when its content was
// last saved.
type QueueItem struct {
	NodeState
	Updated time.Time
}

// queueOrder - the queue's sort columns (QueueSort.Key) as SQL: the
// item's last save, its type's name, its author's name, its title.
var queueOrder = map[string]string{
	QueueSortUpdated: "n.updated_at",
	QueueSortType:    "COALESCE(t.label, n.type)",
	QueueSortAuthor:  "COALESCE(u.username, '')",
	QueueSortTitle:   "n.title",
}

// InState - a page of the items in state in sort's order (an unknown key
// - the last saved first), with how many there are.
func (r *Repo) InState(ctx context.Context, state string, sort QueueSort, limit, offset int) ([]QueueItem, int, error) {
	from := ` FROM workflow_node_states s JOIN nodes n ON n.id = s.nid
		LEFT JOIN content_types t ON t.machine_name = n.type
		LEFT JOIN users u ON u.id = n.created_by
		WHERE s.state = ?`
	var total int
	if err := r.db.QueryRowContext(ctx, modulekit.Rebind("SELECT COUNT(*)"+from), state).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting the items in %s: %w", state, err)
	}
	col, ok := queueOrder[sort.Key]
	if !ok {
		col, sort.Desc = queueOrder[QueueSortUpdated], true
	}
	dir := " ASC"
	if sort.Desc {
		dir = " DESC"
	}
	rows, err := r.db.QueryContext(ctx, modulekit.Rebind("SELECT s.nid, s.state, s.changed_at, s.changed_by, n.updated_at"+from+
		" ORDER BY "+col+dir+", s.id"+dir+" LIMIT ? OFFSET ?"), state, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing the items in %s: %w", state, err)
	}
	defer func() { _ = rows.Close() }()
	var out []QueueItem
	for rows.Next() {
		var it QueueItem
		if err := rows.Scan(&it.Nid, &it.State, &it.ChangedAt, &it.ChangedBy, &it.Updated); err != nil {
			return nil, 0, fmt.Errorf("listing the items in %s: %w", state, err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("listing the items in %s: %w", state, err)
	}
	return out, total, nil
}

// Counts - the number of items in each state.
func (r *Repo) Counts(ctx context.Context) (map[string]int, error) {
	var rows []struct {
		State string `json:"state"`
		Count int    `json:"count"`
	}
	if err := r.client(ctx).NodeState.Query().GroupBy(nodestate.FieldState).Aggregate(generated.Count()).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("counting the items by state: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.State] = row.Count
	}
	return out, nil
}

// joinStates - the states the items of a type joining the workflow get:
// the published ones Published ("" - none such: Initial), those on
// moderation Waiting, the rest Initial.
type joinStates struct{ Published, Initial, Waiting string }

// Backfill records a state for the items of types without one (a type
// joining the workflow) by states, and returns those on moderation: their
// publishing is to follow their state.
func (r *Repo) Backfill(ctx context.Context, types []string, states joinStates, now time.Time) ([]int, error) {
	if len(types) == 0 {
		return nil, nil
	}
	args := []any{}
	marks := ""
	for i, t := range types {
		if i > 0 {
			marks += ", "
		}
		marks += "?"
		args = append(args, t)
	}
	q := `SELECT id, status, on_moderation FROM nodes n WHERE type IN (` + marks + `) AND parent_id IS NULL
		AND NOT EXISTS (SELECT 1 FROM workflow_node_states s WHERE s.nid = n.id)`
	items, err := r.unstated(ctx, q, args)
	if err != nil {
		return nil, err
	}
	c := r.client(ctx)
	var moderated []int
	for _, it := range items {
		state := states.Initial
		switch {
		case it.moderation == 1:
			state = states.Waiting
			moderated = append(moderated, it.id)
		case it.status == 1 && states.Published != "":
			state = states.Published
		}
		if err := c.NodeState.Create().SetNid(it.id).SetState(state).SetChangedAt(now).Exec(ctx); err != nil {
			return nil, fmt.Errorf("the state of node %d: %w", it.id, err)
		}
	}
	return moderated, nil
}

// unstatedItem - an item without a state: its id, publishing, moderation.
type unstatedItem struct{ id, status, moderation int }

// unstated - the items q lists (id, status, on_moderation).
func (r *Repo) unstated(ctx context.Context, q string, args []any) ([]unstatedItem, error) {
	rows, err := r.db.QueryContext(ctx, modulekit.Rebind(q), args...)
	if err != nil {
		return nil, fmt.Errorf("listing the items without a state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []unstatedItem
	for rows.Next() {
		var it unstatedItem
		if err := rows.Scan(&it.id, &it.status, &it.moderation); err != nil {
			return nil, fmt.Errorf("listing the items without a state: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing the items without a state: %w", err)
	}
	return items, nil
}

// States - the recorded states of the items among ids, by id.
func (r *Repo) States(ctx context.Context, ids []int) (map[int]NodeState, error) {
	out := make(map[int]NodeState, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.client(ctx).NodeState.Query().Where(nodestate.NidIn(ids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading the states of nodes: %w", err)
	}
	for _, row := range rows {
		out[row.Nid] = NodeState{Nid: row.Nid, State: row.State, ChangedAt: row.ChangedAt, ChangedBy: row.ChangedBy}
	}
	return out, nil
}

// Lock - nid's lock (false - none).
func (r *Repo) Lock(ctx context.Context, nid int) (Lock, bool, error) {
	row, err := r.client(ctx).Lock.Query().Where(lock.Nid(nid)).Only(ctx)
	if generated.IsNotFound(err) {
		return Lock{}, false, nil
	}
	if err != nil {
		return Lock{}, false, fmt.Errorf("loading the lock of node %d: %w", nid, err)
	}
	return lockOf(row), true, nil
}

// Locks - the locks of the items among ids active since since, by id.
func (r *Repo) Locks(ctx context.Context, ids []int, since time.Time) (map[int]Lock, error) {
	out := make(map[int]Lock, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.client(ctx).Lock.Query().Where(lock.NidIn(ids...), lock.TouchedAtGTE(since)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading the locks of nodes: %w", err)
	}
	for _, row := range rows {
		out[row.Nid] = lockOf(row)
	}
	return out, nil
}

// AcquireLock locks nid for uid at now - its own lock touched, a lock
// inactive since before since taken over - unless another's active lock
// holds it: the lock holding nid after.
func (r *Repo) AcquireLock(ctx context.Context, nid, uid int, now, since time.Time) (Lock, error) {
	c := r.client(ctx)
	n, err := c.Lock.Update().Where(lock.Nid(nid), lock.UID(uid)).SetTouchedAt(now).Save(ctx)
	if err == nil && n == 0 {
		n, err = c.Lock.Update().Where(lock.Nid(nid), lock.TouchedAtLT(since)).SetUID(uid).SetLockedAt(now).SetTouchedAt(now).Save(ctx)
	}
	if err == nil && n == 0 {
		// None yet; another's created meanwhile - theirs.
		if err = c.Lock.Create().SetNid(nid).SetUID(uid).SetLockedAt(now).SetTouchedAt(now).Exec(ctx); generated.IsConstraintError(err) {
			err = nil
		}
	}
	if err != nil {
		return Lock{}, fmt.Errorf("locking node %d: %w", nid, err)
	}
	l, _, err := r.Lock(ctx, nid)
	return l, err
}

// TakeLock locks nid for uid at now, whoever held it.
func (r *Repo) TakeLock(ctx context.Context, nid, uid int, now time.Time) error {
	c := r.client(ctx)
	if _, err := c.Lock.Delete().Where(lock.Nid(nid)).Exec(ctx); err != nil {
		return fmt.Errorf("taking over the lock of node %d: %w", nid, err)
	}
	if err := c.Lock.Create().SetNid(nid).SetUID(uid).SetLockedAt(now).SetTouchedAt(now).Exec(ctx); err != nil {
		return fmt.Errorf("taking over the lock of node %d: %w", nid, err)
	}
	return nil
}

// ReleaseLock removes uid's lock of nid (none - nothing to do).
func (r *Repo) ReleaseLock(ctx context.Context, nid, uid int) error {
	if _, err := r.client(ctx).Lock.Delete().Where(lock.Nid(nid), lock.UID(uid)).Exec(ctx); err != nil {
		return fmt.Errorf("releasing the lock of node %d: %w", nid, err)
	}
	return nil
}

// lockOf - row as a Lock.
func lockOf(row *generated.Lock) Lock {
	return Lock{Nid: row.Nid, UID: row.UID, LockedAt: row.LockedAt, TouchedAt: row.TouchedAt}
}

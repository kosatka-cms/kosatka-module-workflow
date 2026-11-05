package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
)

// lockPath - the item's lock's actions: POST - still editing (the form's
// heartbeat), …/release - the form closed, …/take - taking it over.
const lockPath = "/workflow/lock/"

// Lock - an item edited by UID in its form since LockedAt, last active
// at TouchedAt (workflow_locks).
type Lock struct {
	Nid       int
	UID       int
	LockedAt  time.Time
	TouchedAt time.Time
}

// lockNote - an item's lock as its form shows it: Mine - the user's (the
// form keeps it while active, LockMinutes); else Who holds it since when,
// whether the user MayTake it over.
type lockNote struct {
	Nid     int
	Mine    bool
	Who     string
	Since   time.Time
	MayTake bool
	Minutes int
}

// lockSince - the oldest activity keeping a lock now (false - items
// aren't locked).
func (w *Workflow) lockSince(ctx context.Context) (time.Time, int, bool, error) {
	cfg, err := w.Config(ctx)
	if err != nil || cfg.LockMinutes <= 0 {
		return time.Time{}, 0, false, err
	}
	return w.now().Add(-time.Duration(cfg.LockMinutes) * time.Minute), cfg.LockMinutes, true, nil
}

// lockFor locks nid for user while nobody else edits it: its lock after
// (nil - items aren't locked).
func (w *Workflow) lockFor(ctx context.Context, nid int, user *modulekit.User) (*Lock, int, error) {
	since, minutes, on, err := w.lockSince(ctx)
	if err != nil || !on || user == nil {
		return nil, 0, err
	}
	l, err := w.repo.AcquireLock(ctx, nid, user.ID, w.now(), since)
	if err != nil {
		return nil, 0, err
	}
	return &l, minutes, nil
}

// heldByOther - the active lock of nid by someone but user (nil - none).
func (w *Workflow) heldByOther(ctx context.Context, nid int, user *modulekit.User) (*Lock, error) {
	since, _, on, err := w.lockSince(ctx)
	if err != nil || !on {
		return nil, err
	}
	l, ok, err := w.repo.Lock(ctx, nid)
	if err != nil || !ok || l.TouchedAt.Before(since) || (user != nil && l.UID == user.ID) {
		return nil, err
	}
	return &l, nil
}

// lockTarget - the item of a lock request and its editor: false - the
// answer is given (no such item, not the workflow's, not editable).
func (c *Controller) lockTarget(w http.ResponseWriter, r *http.Request) (*modulekit.Node, *modulekit.User, bool) {
	ctx := r.Context()
	nid, _ := strconv.Atoi(r.PathValue("nid"))
	user, err := c.GetCurrentUser(r)
	if err != nil || user == nil {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return nil, nil, false
	}
	node, err := c.nodes.ByID(ctx, nid)
	if err != nil {
		c.RenderLoadFailure(w, r, err, "loading the locked item", "nid", nid)
		return nil, nil, false
	}
	if !c.w.Handles(ctx, string(node.Type)) || !c.AuthService().CanEditNode(ctx, node, user) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return nil, nil, false
	}
	return node, user, true
}

// lockState - what the form's heartbeat learns: whether the lock is the
// user's, else who holds it.
type lockState struct {
	Mine bool   `json:"mine"`
	Who  string `json:"who,omitempty"`
}

// KeepLock handles POST /workflow/lock/{nid}: the form still edited - its
// lock kept, or got again if free; JSON lockState.
func (c *Controller) KeepLock(w http.ResponseWriter, r *http.Request) {
	node, user, ok := c.lockTarget(w, r)
	if !ok {
		return
	}
	l, _, err := c.w.lockFor(r.Context(), node.ID, user)
	if err != nil {
		c.LogFailure(err, "keeping an item's lock", "nid", node.ID)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	state := lockState{Mine: true}
	if l != nil && l.UID != user.ID {
		state = lockState{Who: userName(r.Context(), c.users, l.UID)}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

// ReleaseLock handles POST /workflow/lock/{nid}/release: the form closed
// unsaved - the user's lock removed.
func (c *Controller) ReleaseLock(w http.ResponseWriter, r *http.Request) {
	node, user, ok := c.lockTarget(w, r)
	if !ok {
		return
	}
	if err := c.w.repo.ReleaseLock(r.Context(), node.ID, user.ID); err != nil {
		c.LogFailure(err, "releasing an item's lock", "nid", node.ID)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TakeLock handles POST /workflow/lock/{nid}/take: the lock taken over
// (PermTakeOver), the form opened.
func (c *Controller) TakeLock(w http.ResponseWriter, r *http.Request) {
	node, user, ok := c.lockTarget(w, r)
	if !ok {
		return
	}
	if !c.AuthService().HasPermission(r.Context(), user, manifest.PermTakeOver) {
		c.RenderForbidden(w, r)
		return
	}
	if err := c.w.repo.TakeLock(r.Context(), node.ID, user.ID, c.w.now()); err != nil {
		c.LogFailure(err, "taking over an item's lock", "nid", node.ID)
	}
	http.Redirect(w, r, "/node/"+strconv.Itoa(node.ID)+"/edit", http.StatusSeeOther)
}

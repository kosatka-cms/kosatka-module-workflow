package workflow

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
	"github.com/kosatka-cms/kosatka/pkg/i18n"
)

// formData - the workflow's part of an item's form: its state, the
// transitions the user may make, its editorial history (nil - a new
// item), its lock (nil - items aren't locked, a new item).
type formData struct {
	State       State
	Transitions []Transition
	History     *editorialHistory
	Lock        *lockNote
}

// users - the users the module names.
type users interface {
	ByID(ctx context.Context, id int) (*modulekit.User, error)
	ByIDs(ctx context.Context, ids []int) (map[int]modulekit.User, error)
}

// nodeForm - the workflow in the content form (modulekit.NodeWorkflow).
type nodeForm struct {
	w     *Workflow
	nodes modulekit.NodesRepository
	users users
	auth  modulekit.AuthService
}

// workflow - the core's hook.
func (f nodeForm) workflow() modulekit.NodeWorkflow {
	return modulekit.NodeWorkflow{Module: manifest.Name, Handles: f.w.Handles, Load: f.load, Validate: f.validate, Apply: f.apply}
}

// stateOf - node's state (a new item's - the initial one).
func (f nodeForm) stateOf(ctx context.Context, node *modulekit.Node) (State, error) {
	if node == nil {
		cfg, err := f.w.Config(ctx)
		return cfg.Initial(), err
	}
	return f.w.StateOf(ctx, *node)
}

func (f nodeForm) load(ctx context.Context, _ string, node *modulekit.Node, user *modulekit.User) (any, error) {
	state, err := f.stateOf(ctx, node)
	if err != nil {
		return nil, err
	}
	allowed, err := f.w.Allowed(ctx, user, state.Name)
	if err != nil {
		return nil, err
	}
	data := formData{State: state, Transitions: allowed}
	if node != nil {
		// The form locks the item, or shows who edits it - read-only.
		l, minutes, err := f.w.lockFor(ctx, node.ID, user)
		if err != nil {
			return nil, err
		}
		if l != nil {
			data.Lock = &lockNote{Nid: node.ID, Mine: l.UID == user.ID, Minutes: minutes}
			if !data.Lock.Mine {
				data.Lock.Who, data.Lock.Since = f.userName(ctx, l.UID), l.LockedAt
				data.Lock.MayTake = f.auth != nil && f.auth.HasPermission(ctx, user, manifest.PermTakeOver)
				data.Transitions = nil
			}
		}
		h, err := f.w.historyOf(ctx, *node, state, f.users)
		if err != nil {
			return nil, err
		}
		data.History = &h
	}
	return data, nil
}

// userName - the name of user uid ("" - none such).
func (f nodeForm) userName(ctx context.Context, uid int) string {
	return userName(ctx, f.users, uid)
}

// userName - the name of user uid among users ("" - none such).
func userName(ctx context.Context, users users, uid int) string {
	if uid <= 0 || users == nil {
		return ""
	}
	u, err := users.ByID(ctx, uid)
	if err != nil || u == nil {
		return ""
	}
	return u.Username
}

// transitionOf - the transition submitted (workflow_transition) from
// state, if user may make it: nil - none submitted; an error message -
// one not available.
func (f nodeForm) transitionOf(ctx context.Context, r *http.Request, state State, user *modulekit.User) (*Transition, string, error) {
	raw := r.FormValue("workflow_transition")
	if raw == "" {
		return nil, "", nil
	}
	id, _ := strconv.Atoi(raw)
	allowed, err := f.w.Allowed(ctx, user, state.Name)
	if err != nil {
		return nil, "", err
	}
	i := slices.IndexFunc(allowed, func(t Transition) bool { return t.ID == id })
	if i < 0 {
		return nil, i18n.T(ctx, "This transition isn't available: the content's state or your role changed."), nil
	}
	return &allowed[i], "", nil
}

func (f nodeForm) validate(ctx context.Context, _ string, node *modulekit.Node, user *modulekit.User, r *http.Request) string {
	state, err := f.stateOf(ctx, node)
	if err != nil {
		return i18n.T(ctx, "The workflow could not be loaded.")
	}
	if node != nil {
		// Another's form holds the item: theirs would be overwritten.
		l, err := f.w.heldByOther(ctx, node.ID, user)
		if err != nil {
			return i18n.T(ctx, "The workflow could not be loaded.")
		}
		if l != nil {
			return i18n.T(ctx, "@name is editing this item now: your changes aren't saved.", "@name", f.userName(ctx, l.UID))
		}
	}
	t, msg, err := f.transitionOf(ctx, r, state, user)
	switch {
	case err != nil:
		return i18n.T(ctx, "The workflow could not be loaded.")
	case msg != "":
		return msg
	case t != nil && t.Comment == manifest.CommentRequired && strings.TrimSpace(r.FormValue("workflow_comment")) == "":
		return i18n.T(ctx, "Write a comment for \"@transition\".", "@transition", t.Label)
	}
	return ""
}

func (f nodeForm) apply(ctx context.Context, n modulekit.Node, user *modulekit.User, r *http.Request, isNew bool) error {
	node, err := f.nodes.ByID(ctx, n.ID)
	if err != nil {
		return fmt.Errorf("loading node %d: %w", n.ID, err)
	}
	state, err := f.w.StateOf(ctx, *node)
	if err != nil {
		return err
	}
	uid := 0
	if user != nil {
		uid = user.ID
	}
	if isNew {
		// A new item starts in the initial state (its publishing too).
		if err := f.w.repo.SetNodeState(ctx, node.ID, state.Name, uid, f.w.now()); err != nil {
			return err
		}
		if err := f.w.publish(ctx, node.ID, state); err != nil {
			return err
		}
	} else if err := f.w.repo.ReleaseLock(ctx, node.ID, uid); err != nil {
		// Saved: the form no longer holds it.
		return err
	}
	t, msg, err := f.transitionOf(ctx, r, state, user)
	if err != nil {
		return err
	}
	if msg != "" || t == nil {
		return nil
	}
	comment := strings.TrimSpace(r.FormValue("workflow_comment"))
	if t.Comment == manifest.CommentNone {
		comment = ""
	}
	return f.w.Make(ctx, *node, state, *t, user, comment)
}

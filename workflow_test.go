package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/kosatka-cms/kosatka/modulekit"
)

// fullAuth - the users with full access: those named "root".
type fullAuth struct{ modulekit.AuthService }

func (fullAuth) HasPermission(_ context.Context, u *modulekit.User, perm string) bool {
	return u != nil && u.Username == "root" && perm == modulekit.PermFullAccess
}

// TestAllowed - a transition from the state for a role the user has; all
// of them with full access; none for a guest; a state's mark and the
// config's initial and first published states.
func TestAllowed(t *testing.T) {
	cfg := Config{
		States: []State{{Name: "draft", Initial: true}, {Name: "review"}, {Name: "live", Published: true, Color: "green"}},
		Transitions: []Transition{
			{ID: 1, From: []string{"draft"}, To: "review", Roles: []string{modulekit.RoleUser}},
			{ID: 2, From: []string{"review"}, To: "live", Roles: []string{"editor"}},
			{ID: 3, From: []string{"review"}, To: "draft", Roles: []string{"editor"}},
		},
	}
	w := &Workflow{auth: fullAuth{}, cfg: &cfg}
	ctx := context.Background()
	ids := func(u *modulekit.User, state string) []int {
		ts, err := w.Allowed(ctx, u, state)
		if err != nil {
			t.Fatal(err)
		}
		var out []int
		for _, tr := range ts {
			out = append(out, tr.ID)
		}
		return out
	}
	author := &modulekit.User{ID: 2, Username: "author"}
	editor := &modulekit.User{ID: 3, Username: "editor", Role: &modulekit.Role{Name: "editor"}}
	root := &modulekit.User{ID: 1, Username: "root"}
	if got := ids(author, "draft"); len(got) != 1 || got[0] != 1 {
		t.Errorf("the author from draft: %v", got)
	}
	if got := ids(author, "review"); len(got) != 0 {
		t.Errorf("the author from review: %v", got)
	}
	if got := ids(editor, "review"); len(got) != 2 {
		t.Errorf("the editor from review: %v", got)
	}
	if got := ids(root, "review"); len(got) != 2 {
		t.Errorf("full access from review: %v", got)
	}
	if got := ids(nil, "draft"); len(got) != 0 {
		t.Errorf("a guest: %v", got)
	}
	if cfg.Initial().Name != "draft" || cfg.FirstPublished() != "live" || cfg.States[2].Hex() != "#1d7a46" || cfg.States[0].Hex() != "#8a959c" {
		t.Errorf("the config's helpers: %q %q", cfg.Initial().Name, cfg.FirstPublished())
	}
	if defaultQueueState(cfg) != "review" {
		t.Errorf("the queue's first tab: %q", defaultQueueState(cfg))
	}
}

func TestStateMark(t *testing.T) {
	got := string(stateMark(State{Label: "<In review>", Color: "amber"}))
	if !strings.Contains(got, "&lt;In review&gt;") || strings.Contains(got, "<In") {
		t.Errorf("the label isn't escaped: %s", got)
	}
	if !strings.Contains(got, (State{Color: "amber"}).Hex()) {
		t.Errorf("the mark hasn't its colour: %s", got)
	}
}

// TestWaiting - an item on moderation's state: the first neither initial
// nor published, else the initial one.
func TestWaiting(t *testing.T) {
	cfg := Config{States: []State{{Name: "draft", Initial: true}, {Name: "live", Published: true}, {Name: "review"}}}
	if got := cfg.Waiting().Name; got != "review" {
		t.Errorf("waiting: %q", got)
	}
	cfg.States = cfg.States[:2]
	if got := cfg.Waiting().Name; got != "draft" {
		t.Errorf("without one: %q", got)
	}
}

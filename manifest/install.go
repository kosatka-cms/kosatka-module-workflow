package manifest

import (
	"context"
	"fmt"

	"github.com/kosatka-cms/kosatka-module-workflow/ent/generated"
	"github.com/kosatka-cms/kosatka/modulekit/manifestkit"
	"github.com/kosatka-cms/kosatka/pkg/i18n"
)

// The default states (their machine names).
const (
	StateDraft     = "draft"
	StateReview    = "review"
	StateApproved  = "approved"
	StatePublished = "published"
	StateArchived  = "archived"
)

// The comment a transition asks for.
const (
	CommentOptional = "optional"
	CommentRequired = "required"
	CommentNone     = "none"
)

// install writes the default workflow: draft → in review → approved →
// published → archived; signed-in users send to review, the rest only
// whoever has full access until the site gives roles. No content type
// follows it yet.
func install(ctx context.Context, env manifestkit.InstallEnv) error {
	c := generated.NewClient(generated.Driver(manifestkit.Driver(env)))
	states := []struct {
		name, label, color string
		published, initial bool
	}{
		{StateDraft, i18n.N("Draft"), "gray", false, true},
		{StateReview, i18n.N("In review"), "amber", false, false},
		{StateApproved, i18n.N("Approved"), "blue", false, false},
		{StatePublished, i18n.N("Published"), "green", true, false},
		{StateArchived, i18n.N("Archived"), "graphite", false, false},
	}
	for i, s := range states {
		if err := c.WorkflowState.Create().SetMachineName(s.name).SetLabel(s.label).SetColor(s.color).
			SetPublished(s.published).SetInitial(s.initial).SetWeight(i * 10).Exec(ctx); err != nil {
			return fmt.Errorf("the workflow's state %s: %w", s.name, err)
		}
	}
	transitions := []struct {
		from      []string
		to, label string
		roles     []string
		comment   string
	}{
		{[]string{StateDraft}, StateReview, i18n.N("Send to review"), []string{manifestkit.RoleUser}, CommentOptional},
		{[]string{StateReview}, StateDraft, i18n.N("Return to the author"), nil, CommentRequired},
		{[]string{StateReview}, StateApproved, i18n.N("Approve"), nil, CommentOptional},
		{[]string{StateApproved, StateReview}, StatePublished, i18n.N("Publish"), nil, CommentNone},
		{[]string{StatePublished}, StateArchived, i18n.N("Archive"), nil, CommentNone},
		{[]string{StateArchived}, StateDraft, i18n.N("Back to work"), nil, CommentNone},
	}
	for i, t := range transitions {
		roles := t.roles
		if roles == nil {
			roles = []string{}
		}
		if err := c.WorkflowTransition.Create().SetFromStates(t.from).SetToState(t.to).SetLabel(t.label).
			SetRoles(roles).SetComment(t.comment).SetWeight(i * 10).Exec(ctx); err != nil {
			return fmt.Errorf("the workflow's transition to %s: %w", t.to, err)
		}
	}
	return c.WorkflowSettings.Create().SetID(1).SetContentTypes([]string{}).Exec(ctx)
}

//go:build dbintegration

package manifest_test

import (
	"context"
	"slices"
	"testing"

	workflow "github.com/kosatka-cms/kosatka-module-workflow"
	"github.com/kosatka-cms/kosatka/modulekit"
	"github.com/kosatka-cms/kosatka/modulekit/testkit"
)

// TestInstall - the default workflow: signed-in users send to review,
// the other transitions are the roles' with full access (the core's
// administrator), named in the settings.
func TestInstall(t *testing.T) {
	testkit.SchemaDB(t, func(t *testing.T, db testkit.DB) {
		cfg, err := workflow.NewWorkflow(workflow.NewRepo(db.DB), nil, nil).Config(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Transitions) != 6 {
			t.Fatalf("transitions: %+v", cfg.Transitions)
		}
		for _, tr := range cfg.Transitions {
			want := []string{"admin"}
			if tr.To == "review" {
				want = []string{modulekit.RoleUser}
			}
			if !slices.Equal(tr.Roles, want) {
				t.Errorf("%s: roles %v, want %v", tr.Label, tr.Roles, want)
			}
		}
	})
}

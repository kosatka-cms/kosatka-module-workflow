package workflow

import (
	"context"
	"net/http"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
)

// userSettingsForm - the module's section of a user's profile form.
type userSettingsForm struct {
	Mail bool
}

// userSettingsSection - whether the user gets the module's e-mails
// (their profile form, user_settings_workflow): on unless turned off.
func userSettingsSection(repo *Repo) modulekit.UserSettingsSection {
	return modulekit.UserSettingsSection{
		Module: manifest.Name,
		Load: func(ctx context.Context, u modulekit.User) (any, error) {
			off, err := repo.MailOff(ctx, []int{u.ID})
			return userSettingsForm{Mail: !off[u.ID]}, err
		},
		Save: func(ctx context.Context, u modulekit.User, r *http.Request) error {
			// A form without the section (a theme's own) changes nothing.
			if r.FormValue("workflow_user_settings") != "1" {
				return nil
			}
			return repo.SetMail(ctx, u.ID, r.FormValue("workflow_mail") == "1")
		},
	}
}

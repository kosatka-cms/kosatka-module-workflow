package workflow

import (
	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
	"github.com/kosatka-cms/kosatka/pkg/i18n"
)

// Wire - the workflow publishing its content types' items through their
// forms (SetNodeWorkflow), an item's history on its page for who may
// edit it (AddNodePage), its settings and the editorial queue, the
// count of what waits for the user by the queue's menu item; the state's
// filter, field and sort criterion in content Views.
func Wire(d modulekit.Deps) modulekit.Parts {
	repo := NewRepo(d.DB)
	w := NewWorkflow(repo, d.NodeRepo, d.Auth)
	// A transition's e-mails: to the item's author, to the next step's
	// roles; who gets them - the profile's setting.
	w.mail = &mailer{repo: repo, nodes: d.NodeRepo, users: d.Users, roles: d.Roles, auth: d.Auth, site: d.SiteInformation, types: d.ContentTypes,
		mail: d.Mail, logger: d.Logger, baseURL: modulekit.MailBaseURL}
	for _, def := range mailDefs() {
		modulekit.AddMail(def)
	}
	modulekit.AddUserSettingsSection(userSettingsSection(repo))
	modulekit.SetNodeWorkflow(nodeForm{w: w, nodes: d.NodeRepo, users: d.Users, auth: d.Auth}.workflow())
	// The item's page: its state and history for its author and editors
	// while it isn't on the site (node_notice).
	modulekit.AddNodePage(manifest.Name, nodePage{w: w, users: d.Users, auth: d.Auth, logger: d.Logger}.parts)
	c := NewController(modulekit.NewBaseController(d.Base), d.DB, w, d.NodeRepo, d.ContentTypes, d.Users, d.Roles)
	modulekit.AddAdminMenuCount(manifest.Name, manifest.QueuePath, c.waiting, i18n.N("Waiting in the editorial workflow"))
	registerViews(w)
	return modulekit.Parts{Routes: func(r modulekit.Routes) {
		RegisterRoutes(r.Mux, c, r.AdminOnly, modulekit.AuthMiddleware(r.Store))
	}}
}

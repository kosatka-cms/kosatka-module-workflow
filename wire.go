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
	w := NewWorkflow(NewRepo(d.DB), d.NodeRepo, d.Auth)
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

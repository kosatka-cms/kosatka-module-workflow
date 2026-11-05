package workflow

import (
	"net/http"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
)

// RegisterRoutes registers the settings and the queue with its actions
// (behind admin), an item's lock's actions (behind signedIn: its
// editors).
func RegisterRoutes(r modulekit.Mux, c *Controller, admin, signedIn func(http.Handler) http.Handler) {
	r.Handle("GET "+manifest.SettingsPath, admin(http.HandlerFunc(c.Settings)))
	r.Handle("POST "+manifest.SettingsPath, admin(http.HandlerFunc(c.SaveSettings)))
	r.Handle("GET "+manifest.QueuePath, admin(http.HandlerFunc(c.Queue)))
	r.Handle("GET "+manifest.QueuePath+"/{nid}/{tid}", admin(http.HandlerFunc(c.TransitionPage)))
	r.Handle("POST "+manifest.QueuePath+"/{nid}/{tid}", admin(http.HandlerFunc(c.Transition)))
	r.Handle("POST "+lockPath+"{nid}", signedIn(http.HandlerFunc(c.KeepLock)))
	r.Handle("POST "+lockPath+"{nid}/release", signedIn(http.HandlerFunc(c.ReleaseLock)))
	r.Handle("POST "+lockPath+"{nid}/take", signedIn(http.HandlerFunc(c.TakeLock)))
}

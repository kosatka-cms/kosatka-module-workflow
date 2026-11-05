package workflow

import (
	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka-module-workflow/translations"
	"github.com/kosatka-cms/kosatka/modulekit"
)

// The module registers itself when its package is imported: a site's own
// binary imports it ("app module add" writes kosatka_modules.go).
func init() {
	modulekit.Register(modulekit.Registration{
		Module:  manifest.Module(),
		Steps:   manifest.RegisterSteps,
		Wire:    Wire,
		Catalog: translations.FS,
		Assets:  Assets,
	})
}

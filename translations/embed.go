// Package translations holds the workflow module's interface catalogs
// (<lang>.json, see pkg/i18n): the strings only the module's code uses.
package translations

import "embed"

// FS - the module's catalogs.
//
//go:embed *.json
var FS embed.FS

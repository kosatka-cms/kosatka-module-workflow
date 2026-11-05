package workflow

import (
	"context"
	"html/template"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
	"github.com/kosatka-cms/kosatka/pkg/i18n"
)

// The module's criteria of content Views.
const (
	keyState = "workflow.state"
	keySince = "workflow.since"
)

// registerViews adds the workflow's filter (the item's state, one of
// them), field (its state's mark) and sort criterion (since when in its
// state) to content Views.
func registerViews(w *Workflow) {
	modulekit.AddViewFilter(modulekit.ViewFilter{Module: manifest.Name, Key: keyState, Label: i18n.N("Workflow state"),
		Options: func(ctx context.Context) ([]modulekit.ViewOption, error) {
			cfg, err := w.Config(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]modulekit.ViewOption, len(cfg.States))
			for i, s := range cfg.States {
				out[i] = modulekit.ViewOption{Value: s.Name, Label: s.Label}
			}
			return out, nil
		},
		Choices: func(ctx context.Context, ids []int) (map[int]string, error) {
			states, err := w.repo.States(ctx, ids)
			if err != nil {
				return nil, err
			}
			out := make(map[int]string, len(states))
			for id, s := range states {
				out[id] = s.State
			}
			return out, nil
		}})
	modulekit.AddViewField(modulekit.ViewField{Module: manifest.Name, Key: keyState, Label: i18n.N("Workflow state"),
		Render: func(ctx context.Context, ids []int, _ map[string]string) (map[int]template.HTML, error) {
			cfg, err := w.Config(ctx)
			if err != nil {
				return nil, err
			}
			states, err := w.repo.States(ctx, ids)
			if err != nil {
				return nil, err
			}
			out := make(map[int]template.HTML, len(states))
			for id, ns := range states {
				if s, ok := cfg.State(ns.State); ok {
					out[id] = stateMark(s)
				}
			}
			return out, nil
		}})
	modulekit.AddViewSort(modulekit.ViewSort{Module: manifest.Name, Key: keySince, Label: i18n.N("In the workflow state since"),
		Values: func(ctx context.Context, ids []int, _ map[string]string) (map[int]float64, error) {
			states, err := w.repo.States(ctx, ids)
			if err != nil {
				return nil, err
			}
			out := make(map[int]float64, len(states))
			for id, s := range states {
				out[id] = float64(s.ChangedAt.Unix())
			}
			return out, nil
		}})
}

// stateMark - a state's mark in a list: its label on its colour (inline:
// a list renders without the module's stylesheet).
func stateMark(s State) template.HTML {
	return template.HTML(`<span class="workflow-state" style="display:inline-flex;align-items:center;gap:.35em;padding:.1em .6em;border-radius:999px;` +
		`background:color-mix(in srgb,` + s.Hex() + ` 14%,transparent);font-size:.8125em;font-weight:600">` +
		`<span aria-hidden="true" style="width:.5em;height:.5em;border-radius:50%;background:` + s.Hex() + `"></span>` +
		template.HTMLEscapeString(s.Label) + `</span>`)
}

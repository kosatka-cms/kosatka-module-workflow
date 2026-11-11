package workflow

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestSettingsFromForm - the states and transitions as the form lists
// them: a blank row and a deleted one left out, the initial one chosen,
// a transition's states known ones but its own; what's wrong - a code.
func TestSettingsFromForm(t *testing.T) {
	form := url.Values{
		"states_count":  {"4"},
		"state_label_0": {"Draft"}, "state_name_0": {"draft"},
		"state_label_1": {"Published"}, "state_name_1": {"published"}, "state_published_1": {"1"}, "state_color_1": {"green"},
		"state_label_2": {"Old"}, "state_name_2": {"old"}, "state_delete_2": {"1"},
		"state_label_3": {""}, "state_name_3": {""},
		"state_initial":     {"0"},
		"transitions_count": {"2"},
		"t_label_0":         {"Publish"}, "t_from_0": {"draft", "published", "gone"}, "t_to_0": {"published"}, "t_roles_0": {"editor"}, "t_comment_0": {"none"},
		"t_label_1": {""},
	}
	cfg, code := settingsFromForm(form)
	if code != "" {
		t.Fatalf("code %q", code)
	}
	if len(cfg.States) != 2 || !cfg.States[0].Initial || cfg.States[1].Color != "green" || !cfg.States[1].Published || cfg.States[0].Color != "gray" {
		t.Errorf("states: %+v", cfg.States)
	}
	if len(cfg.Transitions) != 1 || len(cfg.Transitions[0].From) != 1 || cfg.Transitions[0].From[0] != "draft" || cfg.Transitions[0].Comment != "none" {
		t.Errorf("transitions: %+v", cfg.Transitions)
	}
	for name, bad := range map[string]url.Values{
		"state":           {"states_count": {"1"}, "state_label_0": {"X"}, "state_name_0": {"Bad Name"}},
		"state-duplicate": {"states_count": {"2"}, "state_label_0": {"A"}, "state_name_0": {"a"}, "state_label_1": {"B"}, "state_name_1": {"a"}},
		"states":          {"states_count": {"0"}},
		"transition":      {"states_count": {"1"}, "state_label_0": {"A"}, "state_name_0": {"a"}, "transitions_count": {"1"}, "t_label_0": {"Go"}, "t_to_0": {"a"}, "t_from_0": {"a"}},
	} {
		if _, code := settingsFromForm(bad); code != name {
			t.Errorf("%s: code %q", name, code)
		}
	}
}

// TestSettingsLocking - the editing lock: off - 0 minutes; on - the
// minutes chosen among lockChoices, else the default.
func TestSettingsLocking(t *testing.T) {
	base := url.Values{"states_count": {"1"}, "state_label_0": {"Draft"}, "state_name_0": {"draft"}}
	for _, c := range []struct {
		locking, minutes string
		want             int
	}{{"", "60", 0}, {"1", "60", 60}, {"1", "7", DefaultLockMinutes}} {
		form := url.Values{"locking": {c.locking}, "lock_minutes": {c.minutes}}
		for k, v := range base {
			form[k] = v
		}
		if cfg, _ := settingsFromForm(form); cfg.LockMinutes != c.want {
			t.Errorf("locking %q, %s minutes: %d, want %d", c.locking, c.minutes, cfg.LockMinutes, c.want)
		}
	}
	if opts := lockMinuteOptions(0); !opts[1].On || opts[1].Minutes != DefaultLockMinutes {
		t.Errorf("off: %+v, want the default chosen", opts)
	}
}

// TestQueueSort - the order a request asks for: the last saved first by
// default, a column's own direction without "order"; a header's address
// sorts by its column, the other way when sorted by it already.
func TestQueueSort(t *testing.T) {
	for query, want := range map[string]QueueSort{
		"":                       {Key: QueueSortUpdated, Desc: true},
		"sort=nonsense":          {Key: QueueSortUpdated, Desc: true},
		"sort=since":             {Key: QueueSortUpdated, Desc: true},
		"sort=updated":           {Key: QueueSortUpdated, Desc: true},
		"sort=updated&order=asc": {Key: QueueSortUpdated},
		"sort=author&order=desc": {Key: QueueSortAuthor, Desc: true},
		"sort=type":              {Key: QueueSortType},
	} {
		r := httptest.NewRequest(http.MethodGet, "/admin/content/workflow?"+query, nil)
		if got := queueSortOf(r); got != want {
			t.Errorf("%q: %+v, want %+v", query, got, want)
		}
	}
	for _, tc := range []struct {
		current QueueSort
		column  string
		want    string
	}{
		{QueueSort{Key: QueueSortTitle}, QueueSortUpdated, "/admin/content/workflow?order=desc&sort=updated&state=review"},
		{QueueSort{Key: QueueSortUpdated, Desc: true}, QueueSortUpdated, "/admin/content/workflow?order=asc&sort=updated&state=review"},
		{QueueSort{Key: QueueSortType}, QueueSortType, "/admin/content/workflow?order=desc&sort=type&state=review"},
		{QueueSort{Key: QueueSortUpdated, Desc: true}, QueueSortType, "/admin/content/workflow?order=asc&sort=type&state=review"},
	} {
		if got := queueSortURL("review", tc.current, tc.column); got != tc.want {
			t.Errorf("%+v %s: %s, want %s", tc.current, tc.column, got, tc.want)
		}
	}
}

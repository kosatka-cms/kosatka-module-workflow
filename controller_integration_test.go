//go:build dbintegration

package workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kosatka-cms/kosatka/modulekit"
	"github.com/kosatka-cms/kosatka/modulekit/testkit"
)

// TestQueueRows - the queue's page of items: each with its content, type
// label and author, read for the page at once; an item whose content is
// gone is left out.
func TestQueueRows(t *testing.T) {
	testkit.SchemaDB(t, func(t *testing.T, db testkit.DB) {
		ctx := context.Background()
		for _, q := range []string{
			`INSERT INTO users (id, username, slug, password_hash) VALUES (1, 'marta', 'users/marta', ''), (2, 'oleg', 'users/oleg', '')`,
			`INSERT INTO nodes (id, type, title, slug, created_by) VALUES (5, 'article', 'Harbour festival', 'news/harbour', 1), (6, 'article', 'Night market', 'news/market', 2)`,
		} {
			if _, err := db.DB.ExecContext(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
		d := testkit.Deps(db)
		c := &Controller{w: NewWorkflow(NewRepo(db.DB), d.NodeRepo, testkit.Auth{}), nodes: d.NodeRepo, users: d.Users}
		rows, err := c.queueRows(ctx, []QueueItem{{NodeState: NodeState{Nid: 6}}, {NodeState: NodeState{Nid: 99}}, {NodeState: NodeState{Nid: 5}}}, map[string]string{"article": "Article"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].Node.Title != "Night market" || rows[0].Author != "oleg" || rows[1].Author != "marta" || rows[1].TypeLabel != "Article" || rows[1].Node.URL() != "/news/harbour" {
			t.Errorf("rows: %+v", rows)
		}
	})
}

// TestInStateSorted - a state's items in the order a column gives: the
// last save (the latest first by default), the type's name, the
// author's name, the title, either way, a page at a time.
func TestInStateSorted(t *testing.T) {
	testkit.SchemaDB(t, func(t *testing.T, db testkit.DB) {
		ctx := context.Background()
		for _, q := range []string{
			`INSERT INTO users (id, username, slug, password_hash) VALUES (1, 'marta', 'users/marta', ''), (2, 'oleg', 'users/oleg', ''), (3, 'anna', 'users/anna', '')`,
			`INSERT INTO content_types (id, machine_name, label, created_by, created_at, updated_at) VALUES (1, 'article', 'Article', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP), (2, 'hero', 'Hero', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			`INSERT INTO nodes (id, type, title, slug, created_by, updated_at) VALUES
				(5, 'hero', 'Bravo', 'heroes/bravo', 2, '2026-11-03 10:00:00'),
				(6, 'article', 'Charlie', 'news/charlie', 3, '2026-11-01 10:00:00'),
				(7, 'article', 'Alpha', 'news/alpha', 1, '2026-11-02 10:00:00')`,
			`INSERT INTO workflow_node_states (id, nid, state, changed_at, changed_by) VALUES
				(1, 5, 'review', '2026-11-10 09:00:00', 0),
				(2, 6, 'review', '2026-11-10 08:00:00', 0),
				(3, 7, 'review', '2026-11-10 10:00:00', 0)`,
		} {
			if _, err := db.DB.ExecContext(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
		repo := NewRepo(db.DB)
		for _, tc := range []struct {
			sort QueueSort
			want string
		}{
			{QueueSort{}, "5,7,6"},
			{QueueSort{Key: QueueSortUpdated, Desc: true}, "5,7,6"},
			{QueueSort{Key: QueueSortUpdated}, "6,7,5"},
			{QueueSort{Key: QueueSortType}, "6,7,5"},
			{QueueSort{Key: QueueSortAuthor}, "6,7,5"},
			{QueueSort{Key: QueueSortAuthor, Desc: true}, "5,7,6"},
			{QueueSort{Key: QueueSortTitle}, "7,5,6"},
		} {
			items, total, err := repo.InState(ctx, "review", tc.sort, 10, 0)
			if err != nil {
				t.Fatal(tc.sort, err)
			}
			var got []string
			for _, it := range items {
				got = append(got, fmt.Sprint(it.Nid))
			}
			if strings.Join(got, ",") != tc.want || total != 3 {
				t.Errorf("%+v: %s (%d), want %s (3)", tc.sort, strings.Join(got, ","), total, tc.want)
			}
		}
		items, _, err := repo.InState(ctx, "review", QueueSort{Key: QueueSortUpdated, Desc: true}, 1, 1)
		if err != nil || len(items) != 1 || items[0].Nid != 7 || items[0].Updated.UTC().Day() != 2 || items[0].ChangedAt.UTC().Hour() != 10 {
			t.Errorf("the second page of one: %+v %v", items, err)
		}
	})
}

// TestJoinKeepsLastSave - a type joining the workflow: its item on
// moderation gets the waiting state, unpublished and off moderation, the
// published one the first state on the site, the unpublished one the
// initial; none of them counts as saved now (their last save stays).
func TestJoinKeepsLastSave(t *testing.T) {
	testkit.SchemaDB(t, func(t *testing.T, db testkit.DB) {
		ctx := context.Background()
		saved := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
		for _, q := range []string{
			`INSERT INTO users (id, username, slug, password_hash) VALUES (1, 'marta', 'users/marta', '')`,
			`INSERT INTO nodes (id, type, title, slug, created_by, status, on_moderation) VALUES
				(5, 'article', 'Waiting', 'news/waiting', 1, 1, 1),
				(6, 'article', 'Out', 'news/out', 1, 1, 0),
				(7, 'article', 'Draft', 'news/draft', 1, 0, 0)`,
		} {
			if _, err := db.DB.ExecContext(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.DB.ExecContext(ctx, modulekit.Rebind(`UPDATE nodes SET updated_at = ?`), saved); err != nil {
			t.Fatal(err)
		}
		d := testkit.Deps(db)
		w := NewWorkflow(NewRepo(db.DB), d.NodeRepo, testkit.Auth{})
		cfg := Config{Types: []string{"article"}, LockMinutes: DefaultLockMinutes, States: []State{
			{Name: "draft", Label: "Draft", Initial: true},
			{Name: "review", Label: "In review", Weight: 1},
			{Name: "published", Label: "Published", Published: true, Weight: 2},
		}}
		if err := w.SaveConfig(ctx, db.DB, cfg); err != nil {
			t.Fatal(err)
		}
		states, err := NewRepo(db.DB).States(ctx, []int{5, 6, 7})
		if err != nil {
			t.Fatal(err)
		}
		if states[5].State != "review" || states[6].State != "published" || states[7].State != "draft" {
			t.Errorf("states: %+v", states)
		}
		rows, err := db.DB.QueryContext(ctx, `SELECT id, status, on_moderation, updated_at FROM nodes ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		want := map[int][2]int{5: {0, 0}, 6: {1, 0}, 7: {0, 0}}
		for rows.Next() {
			var id, status, moderation int
			var updated time.Time
			if err := rows.Scan(&id, &status, &moderation, &updated); err != nil {
				t.Fatal(err)
			}
			if [2]int{status, moderation} != want[id] || !updated.Equal(saved) {
				t.Errorf("node %d: status %d, moderation %d, updated %v (want %v, %v)", id, status, moderation, updated, want[id], saved)
			}
		}
	})
}

//go:build dbintegration

package workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"

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

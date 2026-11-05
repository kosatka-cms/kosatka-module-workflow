//go:build dbintegration

package workflow

import (
	"context"
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
		rows, err := c.queueRows(ctx, []NodeState{{Nid: 6}, {Nid: 99}, {Nid: 5}}, map[string]string{"article": "Article"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].Node.Title != "Night market" || rows[0].Author != "oleg" || rows[1].Author != "marta" || rows[1].TypeLabel != "Article" || rows[1].Node.URL() != "/news/harbour" {
			t.Errorf("rows: %+v", rows)
		}
	})
}

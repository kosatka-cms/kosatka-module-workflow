//go:build dbintegration

package workflow

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
	"github.com/kosatka-cms/kosatka/modulekit/testkit"
	"github.com/kosatka-cms/kosatka/pkg/mailtemplate"
)

// sentMail - an e-mail fakeMail was given.
type sentMail struct {
	key, to string
	data    mailtemplate.TokenData
}

// fakeMail records the e-mails sent.
type fakeMail struct{ sent []sentMail }

func (f *fakeMail) Send(_ context.Context, key, to string, data mailtemplate.TokenData) error {
	f.sent = append(f.sent, sentMail{key, to, data})
	return nil
}

// fakeRoles - the site's roles.
type fakeRoles modulekit.Roles

func (f fakeRoles) Roles(context.Context) (modulekit.Roles, error) { return modulekit.Roles(f), nil }

// fakeSite - the site's information.
type fakeSite struct {
	modulekit.SiteInformationRepository
}

func (fakeSite) Get(context.Context) (*modulekit.SiteInformation, error) {
	return &modulekit.SiteInformation{SiteName: "Harbour", DefaultLanguage: "en"}, nil
}

// TestTransitionMail - the author sends their item to review the first
// time: the reviewers' role gets "waits for you" (not the author, not who
// turned the e-mails off, not every signed-in user); a reviewer returns
// it with a comment: its author gets "your item moved" with the comment,
// the item's form and the queue's tab; sent again, only that reviewer is
// told - once they may no longer make the transitions, the role again;
// nobody is told about a move to a state waiting for nobody.
func TestTransitionMail(t *testing.T) {
	testkit.SchemaDB(t, func(t *testing.T, db testkit.DB) {
		ctx := context.Background()
		for _, q := range []string{
			`INSERT INTO roles (id, name, created_at, updated_at) VALUES (12, 'reviewer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP), (13, 'writer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			`INSERT INTO users (id, username, slug, password_hash, email, status, role_id) VALUES
				(1, 'anna', 'users/anna', '', 'anna@example.com', 1, 13), (2, 'oleg', 'users/oleg', '', 'oleg@example.com', 1, 12),
				(3, 'vera', 'users/vera', '', 'vera@example.com', 1, 12), (4, 'ivan', 'users/ivan', '', 'ivan@example.com', 1, 13),
				(6, 'lena', 'users/lena', '', 'lena@example.com', 1, 12)`,
			`INSERT INTO content_types (id, machine_name, label, created_by, created_at, updated_at) VALUES (1, 'article', 'Article', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			`INSERT INTO nodes (id, type, title, slug, created_by, status) VALUES (5, 'article', 'Harbour festival', 'news/harbour', 1, 0)`,
		} {
			if _, err := db.DB.ExecContext(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
		d := testkit.Deps(db)
		repo := NewRepo(db.DB)
		if err := repo.SetMail(ctx, 3, false); err != nil {
			t.Fatal(err)
		}
		mail := &fakeMail{}
		m := &mailer{repo: repo, nodes: d.NodeRepo, users: d.Users, roles: fakeRoles{{ID: 12, Name: "reviewer"}, {ID: 13, Name: "writer"}},
			site: fakeSite{}, types: d.ContentTypes, mail: mail, baseURL: func() string { return "https://example.com" }}
		draft := State{Name: "draft", Label: "Draft", Initial: true}
		review := State{Name: "review", Label: "In review"}
		cfg := Config{Types: []string{"article"}, States: []State{draft, review, {Name: "published", Label: "Published", Published: true}},
			Transitions: []Transition{
				{From: []string{"draft"}, To: "review", Label: "Send to review", Roles: []string{modulekit.RoleUser}},
				{From: []string{"review"}, To: "draft", Label: "Return to the author", Roles: []string{"reviewer"}},
			}}
		// move - a transition of node 5 as Workflow.Make records it, then its e-mails.
		move := func(tr transition) {
			t.Helper()
			mail.sent = nil
			from := draft.Name
			if tr.To.Name == draft.Name {
				from = review.Name
			}
			if err := repo.AddHistory(ctx, HistoryEntry{Nid: 5, From: from, To: tr.To.Name, UID: tr.By, At: time.Now()}); err != nil {
				t.Fatal(err)
			}
			tr.Node = modulekit.Node{ID: 5}
			if err := m.send(ctx, cfg, tr); err != nil {
				t.Fatal(err)
			}
		}
		to := func() []string {
			var out []string
			for _, s := range mail.sent {
				out = append(out, s.key+" "+s.to)
			}
			return out
		}

		move(transition{By: 1, Action: "Send to review", To: review})
		if got := to(); !slices.Equal(got, []string{MailWaiting + " oleg@example.com", MailWaiting + " lena@example.com"}) {
			t.Fatalf("sent to review: %v", got)
		}
		x := mail.sent[0].data.Extra
		if x["[workflow:title]"] != "Harbour festival" || x["[workflow:type]"] != "Article" || x["[workflow:by]"] != "anna" || x["[workflow:to]"] != "In review" ||
			x["[workflow:comment]"] != "" || x["[workflow:edit-url]"] != "https://example.com/node/5/edit" || x["[workflow:queue-url]"] != "https://example.com"+manifest.QueuePath+"?state=review" {
			t.Errorf("the waiting e-mail's values: %+v", x)
		}

		move(transition{By: 2, Action: "Return to the author", To: draft, Comment: "Add the source."})
		if len(mail.sent) != 1 || mail.sent[0].key != MailAuthor || mail.sent[0].to != "anna@example.com" ||
			mail.sent[0].data.Extra["[workflow:comment]"] != "Comment: Add the source." || mail.sent[0].data.Extra["[workflow:by]"] != "oleg" {
			t.Errorf("returned to the author: %+v", mail.sent)
		}

		// Sent again: only who returned it.
		move(transition{By: 1, Action: "Send to review", To: review})
		if got := to(); !slices.Equal(got, []string{MailWaiting + " oleg@example.com"}) {
			t.Errorf("sent to review again: %v", got)
		}
		// Who returned it is a writer now: the reviewers again.
		if _, err := db.DB.ExecContext(ctx, `UPDATE users SET role_id = 13 WHERE id = 2`); err != nil {
			t.Fatal(err)
		}
		move(transition{By: 1, Action: "Send to review", To: review})
		if got := to(); !slices.Equal(got, []string{MailWaiting + " lena@example.com"}) {
			t.Errorf("sent to review, who returned it no longer a reviewer: %v", got)
		}

		// The author turned the e-mails off: nothing.
		if err := repo.SetMail(ctx, 1, false); err != nil {
			t.Fatal(err)
		}
		move(transition{By: 6, Action: "Return to the author", To: draft})
		if len(mail.sent) != 0 {
			t.Errorf("to an author with the e-mails off: %+v", mail.sent)
		}
		off, err := repo.MailOff(ctx, []int{1, 2, 3})
		if err != nil || !off[1] || off[2] || !off[3] {
			t.Errorf("the e-mails off: %v %v", off, err)
		}
		if err := repo.SetMail(ctx, 1, true); err != nil {
			t.Fatal(err)
		}
		if off, _ := repo.MailOff(ctx, []int{1}); off[1] {
			t.Error("turned on again: still off")
		}
	})
}

// TestWaitingMailFullAccess - no role named in the transitions from the
// state (but every signed-in user's): an item sent there the first time
// is told to the users of the roles with full access; one an editor with
// full access moved on (approved) is in their hands - the others aren't
// told.
func TestWaitingMailFullAccess(t *testing.T) {
	testkit.SchemaDB(t, func(t *testing.T, db testkit.DB) {
		ctx := context.Background()
		for _, q := range []string{
			`INSERT INTO roles (id, name, created_at, updated_at) VALUES (13, 'writer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP), (14, 'chief', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			`INSERT INTO users (id, username, slug, password_hash, email, status, role_id) VALUES
				(1, 'anna', 'users/anna', '', 'anna@example.com', 1, 13), (4, 'ivan', 'users/ivan', '', 'ivan@example.com', 1, 13),
				(5, 'boris', 'users/boris', '', 'boris@example.com', 1, 14), (7, 'root', 'users/root', '', 'root@example.com', 1, 14)`,
			`INSERT INTO content_types (id, machine_name, label, created_by, created_at, updated_at) VALUES (1, 'article', 'Article', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			`INSERT INTO nodes (id, type, title, slug, created_by, status) VALUES (5, 'article', 'Harbour festival', 'news/harbour', 1, 0)`,
		} {
			if _, err := db.DB.ExecContext(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
		d := testkit.Deps(db)
		mail := &fakeMail{}
		repo := NewRepo(db.DB)
		roles := fakeRoles{{ID: 13, Name: "writer"}, {ID: 14, Name: "chief", Permissions: map[string]any{modulekit.PermFullAccess: true}}}
		m := &mailer{repo: repo, nodes: d.NodeRepo, users: d.Users, roles: roles, auth: fullAuth{},
			site: fakeSite{}, types: d.ContentTypes, mail: mail, baseURL: func() string { return "https://example.com" }}
		review := State{Name: "review", Label: "In review"}
		approved := State{Name: "approved", Label: "Approved"}
		cfg := Config{Types: []string{"article"}, States: []State{{Name: "draft", Label: "Draft", Initial: true}, review, approved},
			Transitions: []Transition{
				{From: []string{"draft"}, To: "review", Label: "Send to review", Roles: []string{modulekit.RoleUser}},
				{From: []string{"review"}, To: "draft", Label: "Return to the author", Roles: []string{modulekit.RoleUser}},
				{From: []string{"review"}, To: "approved", Label: "Approve"},
				{From: []string{"approved"}, To: "published", Label: "Publish"},
			}}
		// move - a transition of node 5 as Workflow.Make records it, then its e-mails.
		move := func(by int, from string, to State, action string) []string {
			t.Helper()
			mail.sent = nil
			if err := repo.AddHistory(ctx, HistoryEntry{Nid: 5, From: from, To: to.Name, UID: by, At: time.Now()}); err != nil {
				t.Fatal(err)
			}
			if err := m.send(ctx, cfg, transition{Node: modulekit.Node{ID: 5}, By: by, Action: action, To: to}); err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, s := range mail.sent {
				out = append(out, s.key+" "+s.to)
			}
			return out
		}
		if got := move(1, "draft", review, "Send to review"); !slices.Equal(got, []string{MailWaiting + " boris@example.com", MailWaiting + " root@example.com"}) {
			t.Errorf("sent to review: %v", got)
		}
		if got := move(7, "review", approved, "Approve"); !slices.Equal(got, []string{MailAuthor + " anna@example.com"}) {
			t.Errorf("approved by an editor who may publish: %v", got)
		}
	})
}

// TestNextRoles - the roles told an item waits: those of the
// transitions from its state, but the user and anonymous roles; none for
// the initial state or one on the site.
func TestNextRoles(t *testing.T) {
	cfg := Config{Transitions: []Transition{
		{From: []string{"review"}, To: "draft", Roles: []string{"reviewer", modulekit.RoleUser}},
		{From: []string{"review", "approved"}, To: "published", Roles: []string{"editor", "reviewer", modulekit.RoleAnonymous}},
		{From: []string{"draft"}, To: "review", Roles: []string{"writer"}},
	}}
	if got := nextRoles(cfg, State{Name: "review"}); !slices.Equal(got, []string{"reviewer", "editor"}) {
		t.Errorf("review: %v", got)
	}
	if got := nextRoles(cfg, State{Name: "draft", Initial: true}); got != nil {
		t.Errorf("the initial state: %v", got)
	}
	if got := nextRoles(cfg, State{Name: "approved", Published: true}); got != nil {
		t.Errorf("a state on the site: %v", got)
	}
}

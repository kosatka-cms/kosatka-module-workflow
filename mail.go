package workflow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/kosatka-cms/kosatka-module-workflow/manifest"
	"github.com/kosatka-cms/kosatka/modulekit"
	"github.com/kosatka-cms/kosatka/pkg/i18n"
	"github.com/kosatka-cms/kosatka/pkg/mailtemplate"
	"github.com/kosatka-cms/kosatka/pkg/panicsafe"
)

// The module's e-mails (modulekit.AddMail): to an item's author when
// someone else moved it; to who it waits for when it comes to a state
// neither initial nor on the site (mailer.waitingFor).
const (
	MailAuthor  = manifest.Name + ".author"
	MailWaiting = manifest.Name + ".waiting"
)

// mailPlaceholders - the e-mails' own placeholders: the item, its type,
// who made the transition, its button, the new state, the comment
// ("Comment: …", or nothing), the item's form, the queue's tab.
var mailPlaceholders = []string{"[workflow:title]", "[workflow:type]", "[workflow:by]", "[workflow:action]", "[workflow:to]", "[workflow:comment]", "[workflow:edit-url]", "[workflow:queue-url]"}

// mailTimeout - how long sending a transition's e-mails may take.
const mailTimeout = 2 * time.Minute

// mailDefs - the module's e-mails, their texts kept by the Mail page.
func mailDefs() []modulekit.MailDef {
	placeholders := slices.Concat(modulekit.MailSitePlaceholders, modulekit.MailUserPlaceholders, mailPlaceholders)
	sample := map[string]string{
		"[workflow:title]": "Harbour festival", "[workflow:type]": "Article", "[workflow:by]": "oleg",
		"[workflow:action]": "Return to the author", "[workflow:to]": "Draft", "[workflow:comment]": "Comment: add the source of the photos.",
		"[workflow:edit-url]": "https://example.com/node/5/edit", "[workflow:queue-url]": "https://example.com" + manifest.QueuePath + "?state=review",
	}
	return []modulekit.MailDef{
		{
			Key: MailAuthor, Module: manifest.Name, Weight: 10, Optional: true, Placeholders: placeholders, Sample: sample,
			Label:       i18n.N("Your item moved in the editorial workflow"),
			Description: i18n.N("Someone else made a transition of the author's item - returned it with what to fix, approved, published or archived it; a user may turn it off in their profile."),
			Recipient:   i18n.N("to the item's author"),
			Default: modulekit.MailTemplate{
				Subject: i18n.N("\"[workflow:title]\": [workflow:to]"),
				Body:    i18n.N("Hello, [user:name]!\n\n[workflow:by] moved your item \"[workflow:title]\" to \"[workflow:to]\" ([workflow:action]).\n\n[workflow:comment]\n\nOpen the item: [workflow:edit-url]\n\n[site:name]"),
			},
		},
		{
			Key: MailWaiting, Module: manifest.Name, Weight: 11, Optional: true, Placeholders: placeholders, Sample: sample,
			Label:       i18n.N("An item waits for you in the editorial workflow"),
			Description: i18n.N("An item came to a state neither initial nor on the site (in review, approved): to who moved it before and may take the next step; the first time, to the roles of the transitions from that state, none named - to who has full access. A user may turn it off in their profile."),
			Recipient:   i18n.N("to its editors, the first time to the roles of the next step"),
			Default: modulekit.MailTemplate{
				Subject: i18n.N("\"[workflow:title]\" waits: [workflow:to]"),
				Body:    i18n.N("Hello, [user:name]!\n\n[workflow:by] moved the item \"[workflow:title]\" ([workflow:type]) to \"[workflow:to]\". The next step is yours.\n\n[workflow:comment]\n\nOpen the item: [workflow:edit-url]\nThe editorial queue: [workflow:queue-url]\n\n[site:name]"),
			},
		},
	}
}

// mailer sends a transition's e-mails.
type mailer struct {
	repo  *Repo
	nodes modulekit.NodesRepository
	users modulekit.UserRepository
	roles interface {
		Roles(ctx context.Context) (modulekit.Roles, error)
	}
	auth    modulekit.AuthService
	site    modulekit.SiteInformationRepository
	types   modulekit.ContentTypeService
	mail    mailtemplate.Mailer
	logger  *slog.Logger
	baseURL func() string
}

// transition - one made, for its e-mails.
type transition struct {
	Node    modulekit.Node
	By      int
	Action  string
	To      State
	Comment string
}

// later sends t's e-mails in the background (the transition's request
// doesn't wait for the mail server).
func (m *mailer) later(cfg Config, t transition) {
	if m == nil {
		return
	}
	go func() {
		defer panicsafe.Recover(m.logger, "workflow.mail")
		ctx, cancel := context.WithTimeout(context.Background(), mailTimeout)
		defer cancel()
		if err := m.send(ctx, cfg, t); err != nil && m.logger != nil {
			m.logger.Error("editorial workflow e-mails not sent", "node_id", t.Node.ID, "error", err)
		}
	}()
}

// send - t's e-mails: to the item's author when someone else made it,
// to who the item waits for in the new state (recipients); never to who
// made it, nor to who turned the e-mails off.
func (m *mailer) send(ctx context.Context, cfg Config, t transition) error {
	base := ""
	if m.baseURL != nil {
		base = m.baseURL()
	}
	if base == "" {
		modulekit.LogFallback(m.logger, errors.New("the site URL isn't set"), "editorial workflow e-mails not sent", "node_id", t.Node.ID)
		return nil
	}
	// The item as saved: its title and author (the form's may lack them).
	if m.nodes != nil {
		nodes, err := m.nodes.ByIDs(ctx, []int{t.Node.ID})
		if err != nil {
			return fmt.Errorf("loading the item: %w", err)
		}
		n, ok := nodes[t.Node.ID]
		if !ok {
			return nil
		}
		t.Node = n
	}
	author, waiting, err := m.recipients(ctx, cfg, t)
	if err != nil {
		return err
	}
	if author == nil && len(waiting) == 0 {
		return nil
	}
	site, err := m.site.Get(ctx)
	if err != nil {
		return fmt.Errorf("loading the site: %w", err)
	}
	ctx = i18n.WithLang(ctx, site.DefaultLanguage)
	extra := map[string]string{
		"[workflow:title]": t.Node.Title, "[workflow:type]": m.typeLabel(ctx, string(t.Node.Type)), "[workflow:by]": m.userName(ctx, t.By),
		"[workflow:action]": t.Action, "[workflow:to]": t.To.Label, "[workflow:comment]": "",
		"[workflow:edit-url]":  base + "/node/" + strconv.Itoa(t.Node.ID) + "/edit",
		"[workflow:queue-url]": base + manifest.QueuePath + "?" + url.Values{"state": {t.To.Name}}.Encode(),
	}
	if t.Comment != "" {
		extra["[workflow:comment]"] = i18n.T(ctx, "Comment: @comment", "@comment", t.Comment)
	}
	sendTo := func(key string, u modulekit.User) {
		md := modulekit.UserMailTokens(base, site, &u)
		md.Extra = extra
		modulekit.LogFallback(m.logger, m.mail.Send(ctx, key, u.Email, md), "sending an editorial workflow e-mail", "key", key, "user_id", u.ID, "node_id", t.Node.ID)
	}
	if author != nil {
		sendTo(MailAuthor, *author)
	}
	for _, u := range waiting {
		sendTo(MailWaiting, u)
	}
	return nil
}

// recipients - t's e-mails' recipients, active and with the e-mails on:
// the item's author when someone else made the transition; for a state
// neither initial nor on the site, who it waits for (waitingFor).
func (m *mailer) recipients(ctx context.Context, cfg Config, t transition) (*modulekit.User, []modulekit.User, error) {
	var author *modulekit.User
	authorID := t.Node.CreatedBy.ID
	if authorID > 0 && authorID != t.By {
		u, err := m.users.ByID(ctx, authorID)
		if err != nil && !modulekit.IsNotFound(err) {
			return nil, nil, fmt.Errorf("loading the item's author: %w", err)
		}
		if u != nil && u.Status == modulekit.UserStatusActive {
			author = u
		}
	}
	waiting, err := m.waitingFor(ctx, cfg, t)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int, 0, len(waiting)+1)
	if author != nil {
		ids = append(ids, author.ID)
	}
	for _, u := range waiting {
		ids = append(ids, u.ID)
	}
	off, err := m.repo.MailOff(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	if author != nil && off[author.ID] {
		author = nil
	}
	waiting = slices.DeleteFunc(waiting, func(u modulekit.User) bool { return off[u.ID] })
	return author, waiting, nil
}

// waitingFor - who an item that came to t.To, a state neither initial
// nor on the site, waits for: who made its transitions before and may
// make one from that state now (the editor who returned it, when the
// author sends it again); none such - the item comes for the first time -
// the users of the roles named in the transitions from it (nextRoles),
// none named - of the roles with full access. Never who made t, nor the
// author (they have their own e-mail); active users only.
func (m *mailer) waitingFor(ctx context.Context, cfg Config, t transition) ([]modulekit.User, error) {
	if t.To.Initial || t.To.Published {
		return nil, nil
	}
	authorID := t.Node.CreatedBy.ID
	skip := func(id int) bool { return id <= 0 || id == t.By || id == authorID }
	editors, err := m.editors(ctx, cfg, t, skip)
	if err != nil || len(editors) > 0 {
		return editors, err
	}
	roleNames := nextRoles(cfg, t.To)
	roles, err := m.roles.Roles(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading the roles: %w", err)
	}
	var roleIDs []int
	for _, r := range roles {
		if slices.Contains(roleNames, r.Name) || len(roleNames) == 0 && r.Grants(modulekit.PermFullAccess) {
			roleIDs = append(roleIDs, r.ID)
		}
	}
	users, err := m.users.UsersWithRoles(ctx, roleIDs)
	if err != nil {
		return nil, fmt.Errorf("loading the users of the roles %v: %w", roleIDs, err)
	}
	return slices.DeleteFunc(users, func(u modulekit.User) bool { return skip(u.ID) }), nil
}

// editors - who made the item's transitions before (skip aside), active
// and may make one from t.To now, by id.
func (m *mailer) editors(ctx context.Context, cfg Config, t transition, skip func(int) bool) ([]modulekit.User, error) {
	entries, err := m.repo.History(ctx, t.Node.ID, 0)
	if err != nil {
		return nil, err
	}
	var ids []int
	for _, e := range entries {
		if !skip(e.UID) && !slices.Contains(ids, e.UID) {
			ids = append(ids, e.UID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	users, err := m.users.ByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("loading who made the item's transitions: %w", err)
	}
	slices.Sort(ids)
	var out []modulekit.User
	for _, id := range ids {
		u, ok := users[id]
		if !ok || u.Status != modulekit.UserStatusActive {
			continue
		}
		full := m.auth != nil && m.auth.HasPermission(ctx, &u, modulekit.PermFullAccess)
		if len(cfg.allowedFrom(t.To.Name, modulekit.UserRoleNames(&u), full)) > 0 {
			out = append(out, u)
		}
	}
	return out, nil
}

// nextRoles - the roles to tell an item came to state: those named in
// the transitions from it, for a state neither initial nor on the site
// (one waiting for someone); the user and anonymous roles left out.
func nextRoles(cfg Config, state State) []string {
	if state.Initial || state.Published {
		return nil
	}
	var out []string
	for _, t := range cfg.Transitions {
		if !slices.Contains(t.From, state.Name) {
			continue
		}
		for _, r := range t.Roles {
			if r != modulekit.RoleUser && r != modulekit.RoleAnonymous && !slices.Contains(out, r) {
				out = append(out, r)
			}
		}
	}
	return out
}

// typeLabel - the content type's name ("" - unknown).
func (m *mailer) typeLabel(ctx context.Context, machineName string) string {
	if m.types == nil {
		return machineName
	}
	defs, err := m.types.ContentTypes(ctx)
	if err != nil {
		modulekit.LogFallback(m.logger, err, "loading the content types for an editorial workflow e-mail")
		return machineName
	}
	for _, d := range defs {
		if d.MachineName == machineName {
			return d.Label
		}
	}
	return machineName
}

// userName - user uid's name ("" - none such).
func (m *mailer) userName(ctx context.Context, uid int) string {
	if uid <= 0 {
		return ""
	}
	u, err := m.users.ByID(ctx, uid)
	if err != nil || u == nil {
		return ""
	}
	return u.Username
}

# Editorial workflow

An editorial workflow for [Kosatka](https://github.com/kosatka-cms/kosatka): states of content (draft, in review, approved, published, archived - or your own) and the transitions between them by role - who sends to review, who approves, who publishes - with the editorial queue and editing locks.

The *Editorial workflow* module (`workflow`) lets content go through
review before it is published. An item of a content type following the
workflow is in one of its **states** - a draft, in review, approved,
published, archived - and moves between them by **transitions**: *Send
to review*, *Return to the author*, *Approve*, *Publish*. Each transition
is for some roles. Whether the item is on the site depends on its state.

## Install

A site with modules from outside the engine builds its own binary
(`kosatka.Main`); `app module add` adds this one to it:

```bash
app module add github.com/kosatka-cms/kosatka-module-workflow
go build -o app .
```

Then enable *Editorial workflow* on the site's Modules page: it installs its
tables and starter rows. See [Using modules](https://kosatka-cms.github.io/kosatka/modules/using-modules/).

A new site built with it gets it with `app install -modules workflow,…`
(or `-modules all`).

Enabling it installs the states above and six transitions: those four,
*Archive* (published to archived) and *Back to work* (archived to draft).
Only *Send to review* is given to a role, signed-in users; the others are
for who has full access until you give them roles. No content type
follows the workflow yet.

## Setting it up

**Configuration → Editorial workflow** (`/admin/config/workflow`):

- **States** - a name, a machine name (set once), whether an item in it
  is *on the site*, a colour, which one is *initial* (a new item's). The
  last row adds a state. An item whose state is deleted counts as in the
  initial one (or in the first on the site, if it is published).
- **Transitions** - the states it goes *from* (one or several), the state
  it goes *to*, the *button*'s text, the roles that may make it, and
  whether it asks for a *comment*: optional, required (such as *Return
  to the author*, telling what to fix) or none. Who has full access may
  make every transition. A transition is made on an item of a type that
  follows the workflow, by someone who may edit that item.
- **Content types** - the ones following the workflow. When a type
  joins, its published items get the first state that is on the site,
  those on moderation the first state neither initial nor on the site
  (*In review*) and leave moderation, the others the initial one. A type
  that doesn't follow it publishes as before, by its *New content*
  setting.

The settings page needs the *Access administration (/admin)* permission.

## Published, on moderation and the states

An item is on the site, as before, when it is published and not on
moderation: that is what search, the sitemap and a View's `status`
filter look at. The workflow sets it: a transition to a state that is
*on the site* publishes the item, to another one unpublishes it, and
either takes it off moderation - waiting for review is a state (*In
review*), the editorial queue lists it.

So the type's own ways of publishing don't apply to its items: its
*New content* setting (a new item is in the initial state), the form's
*Published* and *On schedule*, the item page's *Publish* and *Unpublish*,
the content list's bulk publishing and moderation actions (they leave
such items out, saying why), and a schedule set before the type joined
(when its time comes, it is cleared and changes nothing). The moderation
page doesn't get them either.

## In the content form

For a type following the workflow, the form's side card shows the item's
state instead of *Published*, a *Comment* for whoever is next, and a
button per transition the user may make; *Save* keeps the state. The
publishing checkboxes and the schedule aren't there: the transitions
publish and unpublish.

When the last transition left a comment (*Return to the author* with what
to fix), the form shows it over the fields.

## The editorial queue

**Content → Editorial** (`/admin/content/workflow`) lists the items by
state, a tab each with how many there are, opening on the first state
neither initial nor on the site; the longest waiting first, with their
type, author and since when. Each row has the transitions the user may
make from that state: one that requires a comment opens a page for it,
the others are made at once, without one. The number by the menu item is
how many items wait for a transition of yours (in a state neither
initial nor on the site). The queue needs the *Access administration
(/admin)* permission; an editor without it makes the transitions in the
content form.

Every transition is kept, with its comment, in the workflow's own record
(the form shows the latest comment, above). One that publishes or
unpublishes the item is also in the action log and, when its type keeps
one, in the item's history.

## Editing lock

While an item of a type following the workflow is open in its form, it
is locked: another editor opening it sees who edits it and since when,
over a read-only form, and can't save it. The lock is released when the
item is saved or its form closed; by itself after some time without
activity in the form (typing, clicking) - 30 minutes unless set
otherwise.

Who has the **Take over another's editing lock** permission (and full
access) sees *Take over editing* in that notice: the form becomes theirs.
The one who had it learns it in their form, at the latest when saving:
their changes aren't saved, the form comes back with them, read-only, to
copy.

The queue shows by an item who edits it now. **Editing → Editing lock**
in the settings turns locking off or sets the time: 15, 30, 60 or 120
minutes.

## In Views

A content View (see [Views](https://kosatka-cms.github.io/kosatka/guides/views/)) can use the workflow:

- the filter **Workflow state** - the items in one state or in several
  (or with none: of types not following it); exposed, visitors pick the
  state from a list;
- the field **Workflow state** - the state's name on its colour;
- the sort criterion **In the workflow state since** - when the item
  came to its state.

A View lists whatever its filters match, unpublished items too (see
[Choosing the content](https://kosatka-cms.github.io/kosatka/guides/views/#choosing-the-content-filters)),
and its page is open to everyone. So a public View with the state filter
keeps `status` = `1`, unless it shows unpublished items on purpose -
*Approved* ones announced ahead, say. For the editors' list of what
waits for review, use the editorial queue.

## Not there yet

- Editing a published item while the site shows the published version
  until the new one is approved.

## Development

Its strings are translated in `translations/<lang>.json`.

The module is built on the engine's module SDK only
(`github.com/kosatka-cms/kosatka/modulekit`, `pkg/*`).

```bash
go test ./...
# with MySQL and PostgreSQL (the engine's docker-compose.test.yml):
KOSATKA_TEST_MYSQL_DSN="root:test@tcp(127.0.0.1:33306)/?parseTime=true" \
KOSATKA_TEST_POSTGRES_DSN="postgres://postgres:test@127.0.0.1:35432/postgres?sslmode=disable" \
  go test -tags dbintegration ./...
```

A schema change: edit `ent/schema`, `go generate ./ent`, then
`go run ./cmd/migrations -name <name>` (both test databases running) -
it writes the next migration for MySQL and PostgreSQL into
`ent/migrations`; `go run ./cmd/migrations -check` fails while the files
are behind the schema. See [Write a module](https://kosatka-cms.github.io/kosatka/modules/write-a-module/).

## License

MIT

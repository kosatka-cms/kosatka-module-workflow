// Package schema - the workflow module's own ent schema: its states and
// transitions, which content types follow it, each item's state and the
// transitions made.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/kosatka-cms/kosatka/modulekit/schemakit"
)

// WorkflowState - a state of an item: machine_name, label, published -
// the item is on the site in it, color - its mark (a palette name),
// initial - a new item's (one), weight - its order.
type WorkflowState struct{ ent.Schema }

// Annotations of the WorkflowState.
func (WorkflowState) Annotations() []schema.Annotation {
	return []schema.Annotation{schemakit.Table("workflow_states")}
}

// Fields of the WorkflowState.
func (WorkflowState) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").SchemaType(schemakit.MySQLType("int unsigned")),
		field.String("machine_name").MaxLen(64),
		field.String("label").MaxLen(128),
		field.Bool("published").Default(false),
		field.String("color").MaxLen(16).Default("gray"),
		field.Bool("initial").Default(false),
		field.Int("weight").Default(0),
	}
}

// Indexes of the WorkflowState.
func (WorkflowState) Indexes() []ent.Index {
	return []ent.Index{index.Fields("machine_name").Unique().StorageKey("workflow_states_machine_name")}
}

// WorkflowTransition - a move from one of from_states to to_state by the
// button label, for the roles (role names; the full-access ones may make
// any); comment - "optional", "required" or "none".
type WorkflowTransition struct{ ent.Schema }

// Annotations of the WorkflowTransition.
func (WorkflowTransition) Annotations() []schema.Annotation {
	return []schema.Annotation{schemakit.Table("workflow_transitions")}
}

// Fields of the WorkflowTransition.
func (WorkflowTransition) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").SchemaType(schemakit.MySQLType("int unsigned")),
		field.JSON("from_states", []string{}),
		field.String("to_state").MaxLen(64),
		field.String("label").MaxLen(128),
		field.JSON("roles", []string{}),
		field.String("comment").MaxLen(16).Default("optional"),
		field.Int("weight").Default(0),
	}
}

// WorkflowSettings - the module's settings (one row, id 1):
// content_types - the types whose items follow the workflow;
// lock_minutes - how long an item's form stays locked without its
// editor's activity (0 - items aren't locked).
type WorkflowSettings struct{ ent.Schema }

// Annotations of the WorkflowSettings.
func (WorkflowSettings) Annotations() []schema.Annotation {
	return []schema.Annotation{schemakit.Table("workflow_settings")}
}

// Fields of the WorkflowSettings.
func (WorkflowSettings) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").SchemaType(schemakit.MySQLType("int unsigned")),
		field.JSON("content_types", []string{}).Optional(),
		field.Int("lock_minutes").Default(30),
	}
}

// NodeState - an item's state since changed_at, by changed_by (0 -
// nobody: set when its type joined the workflow).
type NodeState struct{ ent.Schema }

// Annotations of the NodeState.
func (NodeState) Annotations() []schema.Annotation {
	return []schema.Annotation{schemakit.Table("workflow_node_states")}
}

// Fields of the NodeState.
func (NodeState) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").SchemaType(schemakit.MySQLType("int unsigned")),
		field.Int("nid").SchemaType(schemakit.MySQLType("int unsigned")),
		field.String("state").MaxLen(64),
		field.Time("changed_at").SchemaType(schemakit.MySQLType("datetime")),
		field.Int("changed_by").SchemaType(schemakit.MySQLType("int unsigned")).Default(0),
	}
}

// Edges of the NodeState.
func (NodeState) Edges() []ent.Edge {
	return []ent.Edge{edge.To("node", Node.Type).Field("nid").Required().Unique().Annotations(entsql.OnDelete(entsql.Cascade))}
}

// Indexes of the NodeState.
func (NodeState) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("nid").Unique().StorageKey("workflow_node_states_nid"),
		index.Fields("state", "changed_at").StorageKey("workflow_node_states_state"),
	}
}

// History - a transition made: an item from from_state to to_state by
// uid at created_at, with comment.
type History struct{ ent.Schema }

// Annotations of the History.
func (History) Annotations() []schema.Annotation {
	return []schema.Annotation{schemakit.Table("workflow_history")}
}

// Fields of the History.
func (History) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").SchemaType(schemakit.MySQLType("int unsigned")),
		field.Int("nid").SchemaType(schemakit.MySQLType("int unsigned")),
		field.String("from_state").MaxLen(64),
		field.String("to_state").MaxLen(64),
		field.Int("uid").SchemaType(schemakit.MySQLType("int unsigned")).Default(0),
		field.Text("comment").SchemaType(schemakit.MySQLType("text")),
		field.Time("created_at").SchemaType(schemakit.MySQLType("datetime")),
	}
}

// Edges of the History.
func (History) Edges() []ent.Edge {
	return []ent.Edge{edge.To("node", Node.Type).Field("nid").Required().Unique().Annotations(entsql.OnDelete(entsql.Cascade))}
}

// Indexes of the History.
func (History) Indexes() []ent.Index {
	return []ent.Index{index.Fields("nid", "created_at").StorageKey("workflow_history_nid")}
}

// Lock - an item edited by uid in its form since locked_at, last active
// at touched_at.
type Lock struct{ ent.Schema }

// Annotations of the Lock.
func (Lock) Annotations() []schema.Annotation {
	return []schema.Annotation{schemakit.Table("workflow_locks")}
}

// Fields of the Lock.
func (Lock) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").SchemaType(schemakit.MySQLType("int unsigned")),
		field.Int("nid").SchemaType(schemakit.MySQLType("int unsigned")),
		field.Int("uid").SchemaType(schemakit.MySQLType("int unsigned")),
		field.Time("locked_at").SchemaType(schemakit.MySQLType("datetime")),
		field.Time("touched_at").SchemaType(schemakit.MySQLType("datetime")),
	}
}

// Edges of the Lock.
func (Lock) Edges() []ent.Edge {
	return []ent.Edge{edge.To("node", Node.Type).Field("nid").Required().Unique().Annotations(entsql.OnDelete(entsql.Cascade))}
}

// Indexes of the Lock.
func (Lock) Indexes() []ent.Index {
	return []ent.Index{index.Fields("nid").Unique().StorageKey("workflow_locks_nid")}
}

// Node - the core's nodes (their id only): an item's state, history and
// lock, deleted with it (ON DELETE CASCADE). The module reads them through the
// core.
type Node struct{ ent.Schema }

// Annotations of the Node.
func (Node) Annotations() []schema.Annotation {
	return []schema.Annotation{schemakit.Table("nodes")}
}

// Fields of the Node.
func (Node) Fields() []ent.Field {
	return []ent.Field{field.Int("id").SchemaType(schemakit.MySQLType("int unsigned"))}
}

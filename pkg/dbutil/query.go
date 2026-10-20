package dbutil

// Scope is a single query modifier applied in order by ApplyScopes.
type Scope func(*Query) *Query

// Query is a small, driver-agnostic representation of a read statement: a
// projection, a WHERE clause with positional arguments, an ORDER BY and a
// LIMIT/OFFSET window. It keeps services free of hand-built SQL while the
// repository layer stays on the standard library database driver.
type Query struct {
	Fields  string
	Where   string
	Args    []any
	OrderBy string
	Limit   int
	Offset  int
}

// ApplyScopes folds every scope into the query, later scopes overriding
// earlier ones where they set the same attribute.
func ApplyScopes(opts ...Scope) *Query {
	q := &Query{}
	for _, opt := range opts {
		q = opt(q)
	}
	return q
}

// Where sets the WHERE clause and its positional arguments.
func Where(clause string, args ...any) Scope {
	return func(q *Query) *Query {
		q.Where = clause
		q.Args = args
		return q
	}
}

// Select sets the projected columns for the read query.
func Select(fields string) Scope {
	return func(q *Query) *Query {
		q.Fields = fields
		return q
	}
}

// Order sets the ORDER BY clause.
func Order(clause string) Scope {
	return func(q *Query) *Query {
		q.OrderBy = clause
		return q
	}
}

// Limit sets the maximum number of rows returned.
func Limit(limit int) Scope {
	return func(q *Query) *Query {
		q.Limit = limit
		return q
	}
}

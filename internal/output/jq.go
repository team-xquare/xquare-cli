package output

import (
	"fmt"

	"github.com/itchyny/gojq"
)

// jqResults holds all values emitted by a jq expression.
// Unlike wrapping into a Go slice, this preserves the distinction between
// "jq produced a single array value" and "jq produced multiple separate values".
type jqResults struct {
	values []any
}

// applyGojq runs expr against v and returns a *jqResults.
// Callers must use printJQResults to emit output so that multi-value
// expressions (e.g. ".[]") print one JSON value per line, matching real jq.
func applyGojq(v any, expr string) (any, error) {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("parse jq expression: %w", err)
	}
	iter := q.Run(v)
	var results []any
	for {
		val, ok := iter.Next()
		if !ok {
			break
		}
		if e, ok := val.(error); ok {
			return nil, e
		}
		results = append(results, val)
	}
	return &jqResults{values: results}, nil
}

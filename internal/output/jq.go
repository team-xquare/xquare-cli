package output

import (
	"fmt"

	"github.com/itchyny/gojq"
)

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
		if err, ok := val.(error); ok {
			return nil, err
		}
		results = append(results, val)
	}
	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}

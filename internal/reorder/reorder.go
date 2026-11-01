// Package reorder applies relative moves to owned copies of ordered collections.
package reorder

import (
	"errors"
	"slices"
)

// Move returns a new slice with source placed immediately before or after target.
// Both IDs must still exist, even when source and target are the same item.
func Move[T any](items []T, source, target string, after bool, id func(T) string) ([]T, error) {
	from, to := -1, -1
	for i, item := range items {
		if id(item) == source {
			from = i
		}
		if id(item) == target {
			to = i
		}
	}
	if from < 0 || to < 0 {
		return nil, errors.New("reorder source or target no longer exists")
	}
	result := slices.Clone(items)
	if from == to {
		return result, nil
	}
	item := result[from]
	result = slices.Delete(result, from, from+1)
	if from < to {
		to--
	}
	if after {
		to++
	}
	return slices.Insert(result, to, item), nil
}

package desktop

import (
	"gcode/internal/contracts"
	"strings"
)

// conversationMatches returns the rows whose text holds the query, as the list
// indices the transcript scrolls to. Rows are blocks, so a hit names the block
// it is in rather than the message around it.
func (a *App) conversationMatches(cache *transcriptCache, entries []contracts.Message, rows []conversationRow, query string) []int {
	if query == "" {
		return nil
	}
	query = strings.ToLower(query)
	var result []int
	for index, row := range rows {
		if row.Message < 0 {
			continue
		}
		message := entries[row.Message]
		switch {
		case row.User, row.Bare:
			if messageMatches(message.Blocks, query) {
				result = append(result, index)
			}
		default:
			projected := a.transcriptRowAt(cache, message, row.Block)
			if blockMatches(projected.Block, query) || messageMatches(projected.Activity, query) {
				result = append(result, index)
			}
		}
	}
	return result
}

func messageMatches(blocks []contracts.Block, query string) bool {
	for _, block := range blocks {
		if blockMatches(block, query) {
			return true
		}
	}
	return false
}

func blockMatches(block contracts.Block, query string) bool {
	return strings.Contains(strings.ToLower(block.Text), query)
}

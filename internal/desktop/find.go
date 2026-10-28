package desktop

import (
	"gcode/internal/contracts"
	"strings"
)

func conversationMatches(messages []contracts.Message, query string) []int {
	if query == "" {
		return nil
	}
	query = strings.ToLower(query)
	var result []int
	for i, message := range messages {
		for _, block := range message.Blocks {
			if strings.Contains(strings.ToLower(block.Text), query) {
				result = append(result, i)
				break
			}
		}
	}
	return result
}

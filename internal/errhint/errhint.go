// Package errhint contains small provider-error helpers shared by the agent.
package errhint

import "strings"

// HasStatusCode reports whether haystack contains any of the given HTTP status
// codes as a standalone number — not embedded in a longer digit run like
// "completed in 4290ms" or "request id 14015". It is exported so other
// packages can gate on a status code without reimplementing the boundary check.
func HasStatusCode(haystack string, codes ...string) bool {
	for _, code := range codes {
		for from := 0; ; {
			rel := strings.Index(haystack[from:], code)
			if rel < 0 {
				break
			}
			pos := from + rel
			beforeOK := pos == 0 || !isASCIIDigit(haystack[pos-1])
			end := pos + len(code)
			afterOK := end >= len(haystack) || !isASCIIDigit(haystack[end])
			if beforeOK && afterOK {
				return true
			}
			from = pos + 1
		}
	}
	return false
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

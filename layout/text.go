// Package layout measures content without retaining worksheet or database state.
package layout

import (
	"github.com/mattn/go-runewidth"
	"strings"
)

// LongestLine returns display width using locale-independent glyph rules.
func LongestLine(text string) int {
	longest := 0
	// Keep glyph widths independent of the host's locale. Wide characters take
	// two units; combining marks do not inflate a visible character's width.
	condition := runewidth.Condition{StrictEmojiNeutral: true}
	for line := range strings.SplitSeq(text, "\n") {
		longest = max(longest, condition.StringWidth(line))
	}
	return longest
}

// Preserve catalog labels, including deliberate line breaks. For a single-line
// heading, budget for a balanced two-line display when words allow it.
// CompactHeadingLength budgets a balanced two-line heading when words permit.
func CompactHeadingLength(text string) int {
	length := LongestLine(text)
	if strings.Contains(text, "\n") || length <= 12 {
		return length
	}
	words := strings.Fields(text)
	for split := 1; split < len(words); split++ {
		length = min(length, max(LongestLine(strings.Join(words[:split], " ")), LongestLine(strings.Join(words[split:], " "))))
	}
	return length
}

// LineCount measures wrapping without truncating the original value.
func LineCount(text string, capacity int) int {
	capacity = max(1, capacity)
	lines := 0
	for paragraph := range strings.SplitSeq(text, "\n") {
		used, count := 0, 1
		for _, word := range strings.Fields(paragraph) {
			length := LongestLine(word)
			if used > 0 {
				if used+1+length <= capacity {
					used += 1 + length
					continue
				}
				count++
			}
			// Excel also wraps a token with no spaces when it exceeds the cell.
			count += (length - 1) / capacity
			used = (length-1)%capacity + 1
		}
		lines += count
	}
	return lines
}

package review

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/bartinthefield/buti/internal/but"
)

// Excerpt renders the anchored lines of fd with up to around lines of context on each side, from the anchor's side
// of the diff, as "  41 | text" with the anchored lines marked "> 42 | text". Only lines the diff shows are
// included, and a gap between them is a "  ..." line.
//
// With a nil fd, or when the anchored lines are not in it (an outdated comment), it shows the stored line text at
// the stored line numbers instead, so an agent still sees what the comment was about.
func Excerpt(fd *but.FileDiff, a Anchor, around int) string {
	end := max(a.EndLine, a.Line)
	var lines map[int]diffLine
	if fd != nil {
		lines = sideLines(fd, a.Side)
	}
	if lines == nil || findLines(lines, a).line != a.Line {
		lines = map[int]diffLine{}
		for i, t := range anchorLines(a) {
			lines[a.Line+i] = diffLine{text: t}
		}
		around = 0
	}
	var nums []int
	for n := range lines {
		if n >= a.Line-around && n <= end+around {
			nums = append(nums, n)
		}
	}
	slices.Sort(nums)
	if len(nums) == 0 {
		return ""
	}
	width := len(strconv.Itoa(nums[len(nums)-1]))
	var b strings.Builder
	for i, n := range nums {
		if i > 0 && n != nums[i-1]+1 {
			b.WriteString("  ...\n")
		}
		mark := "  "
		if n >= a.Line && n <= end {
			mark = "> "
		}
		fmt.Fprintf(&b, "%s%*d | %s\n", mark, width, n, lines[n].text)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

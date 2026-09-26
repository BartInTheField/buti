package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

const tabWidth = 4

var chromaStyle = func() *chroma.Style {
	if s := styles.Get("catppuccin-mocha"); s != nil {
		return s
	}
	return styles.Fallback
}()

// segment is a run of text sharing one syntax style.
type segment struct {
	text  string
	style lipgloss.Style
}

// lexerCache holds the lexer matched for each path, nil when none matched.
var lexerCache = map[string]chroma.Lexer{}

// highlight tokenizes text with the lexer for path and returns the segments of each line.
func highlight(path, text string) [][]segment {
	lexer, ok := lexerCache[path]
	if !ok {
		lexer = lexers.Match(path)
		lexerCache[path] = lexer
	}
	if lexer == nil {
		lexer = lexers.Analyse(text)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	lines := [][]segment{nil}
	it, err := lexer.Tokenise(nil, text)
	if err != nil {
		for _, l := range strings.Split(text, "\n") {
			lines = append(lines, []segment{{text: l}})
		}
		return lines[1:]
	}
	for tok := it(); tok != chroma.EOF; tok = it() {
		st := tokenStyle(tok.Type)
		for i, part := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				lines = append(lines, nil)
			}
			if part != "" {
				lines[len(lines)-1] = append(lines[len(lines)-1], segment{text: part, style: st})
			}
		}
	}
	return lines
}

var tokenStyles = map[chroma.TokenType]lipgloss.Style{}

func tokenStyle(t chroma.TokenType) lipgloss.Style {
	if st, ok := tokenStyles[t]; ok {
		return st
	}
	e := chromaStyle.Get(t)
	st := lipgloss.NewStyle()
	if e.Colour.IsSet() {
		st = st.Foreground(lipgloss.Color(e.Colour.String()))
	}
	st = st.Bold(e.Bold == chroma.Yes).Italic(e.Italic == chroma.Yes)
	tokenStyles[t] = st
	return st
}

func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth)) }

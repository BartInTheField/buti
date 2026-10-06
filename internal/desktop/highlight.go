package desktop

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Syntax highlighting uses Chroma, the highlighter the TUI uses (internal/ui/highlight.go), so both pick the same
// lexer for a file. The API returns token classes, not colours: the webview themes them for light and dark.

// highlightReq holds source texts to tokenise, grouped by the file whose name picks the lexer. The diff pane sends
// each hunk's old and new side separately, so every lexer sees coherent source, as the TUI does.
type highlightReq struct {
	Files []struct {
		Path  string   `json:"path"`
		Texts []string `json:"texts"`
	} `json:"files"`
}

// token is [class, text]; an empty class is plain text.
type token [2]string

// highlightedFile has, for each text, its lines, each a list of tokens.
type highlightedFile struct {
	Texts [][][]token `json:"texts"`
}

const highlightMaxBody = 16 << 20

func (s *Server) highlight(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	var req highlightReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, highlightMaxBody)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body", "")
		return
	}
	out := make([]highlightedFile, len(req.Files))
	for i, f := range req.Files {
		out[i].Texts = make([][][]token, len(f.Texts))
		for j, text := range f.Texts {
			out[i].Texts[j] = highlightLines(f.Path, text)
		}
	}
	writeJSON(w, http.StatusOK, struct {
		OK    bool              `json:"ok"`
		Files []highlightedFile `json:"files"`
	}{OK: true, Files: out})
}

// lexerFor matches a lexer on the file name, remembering it per path; nil when none matched.
var lexerCache sync.Map

func lexerFor(path, text string) chroma.Lexer {
	var lexer chroma.Lexer
	if v, ok := lexerCache.Load(path); ok {
		lexer, _ = v.(chroma.Lexer)
	} else {
		lexer = lexers.Match(path)
		lexerCache.Store(path, lexer)
	}
	if lexer == nil {
		lexer = lexers.Analyse(text)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return chroma.Coalesce(lexer)
}

// highlightLines tokenises text and splits it into lines, merging neighbouring tokens of one class.
func highlightLines(path, text string) [][]token {
	if text == "" {
		return [][]token{{}}
	}
	lines := [][]token{{}}
	add := func(cls, part string) {
		if part == "" {
			return
		}
		cur := &lines[len(lines)-1]
		if n := len(*cur); n > 0 && (*cur)[n-1][0] == cls {
			(*cur)[n-1][1] += part
			return
		}
		*cur = append(*cur, token{cls, part})
	}
	it, err := lexerFor(path, text).Tokenise(nil, text)
	if err != nil {
		lines = lines[:0]
		for _, l := range strings.Split(text, "\n") {
			lines = append(lines, []token{{"", l}})
		}
		return lines
	}
	for tok := it(); tok != chroma.EOF; tok = it() {
		cls := tokenClass(tok.Type)
		for i, part := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				lines = append(lines, []token{})
			}
			add(cls, part)
		}
	}
	// Lexers end the source with a newline the text did not have.
	if len(lines) > 1 && len(lines[len(lines)-1]) == 0 && !strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// tokenClass folds Chroma's token types into the few classes the diff pane colours.
func tokenClass(t chroma.TokenType) string {
	switch {
	case t == chroma.KeywordType || t == chroma.NameClass || t == chroma.NameNamespace:
		return "t"
	case t == chroma.KeywordConstant || t == chroma.NameConstant || t == chroma.Literal:
		return "c"
	case t.InCategory(chroma.Keyword):
		return "k"
	case t == chroma.NameFunction || t == chroma.NameFunctionMagic:
		return "f"
	case t == chroma.NameBuiltin || t == chroma.NameBuiltinPseudo:
		return "b"
	case t == chroma.NameTag:
		return "g"
	case t == chroma.NameAttribute || t == chroma.NameDecorator:
		return "a"
	case t.InSubCategory(chroma.LiteralString):
		return "s"
	case t.InSubCategory(chroma.LiteralNumber):
		return "n"
	case t == chroma.CommentPreproc || t == chroma.CommentPreprocFile:
		return "p"
	case t.InCategory(chroma.Comment):
		return "m"
	case t.InCategory(chroma.Operator):
		return "o"
	case t.InCategory(chroma.Punctuation):
		return "u"
	case t == chroma.GenericHeading || t == chroma.GenericSubheading || t == chroma.GenericStrong:
		return "h"
	}
	return ""
}

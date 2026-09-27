package reviewcli

import (
	"strings"
	"testing"

	"github.com/bartinthefield/buti/internal/review"
)

// comment runs `buti review comment args` and returns the stored comment.
func (h *harness) comment(args ...string) review.Comment {
	h.t.Helper()
	out := h.ok(append([]string{"comment"}, args...)...)
	f := strings.Fields(out)
	if len(f) < 2 || f[0] != "commented" {
		h.t.Fatalf("comment printed %q", out)
	}
	c, err := h.store.Get(f[1])
	if err != nil {
		h.t.Fatal(err)
	}
	return c
}

func TestComment(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want review.Anchor
		out  string
	}{
		{
			name: "unassigned by default",
			args: []string{"--file", "README.md", "--line", "5"},
			want: review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Side: review.SideNew, Line: 5, EndLine: 5,
				LineText: "## Usage"},
			out: "on zz README.md:5",
		},
		{
			name: "assigned by default",
			args: []string{"--file", "docs/a.md", "--line", "1"},
			want: review.Anchor{Kind: review.KindAssigned, Branch: "docs", Path: "docs/a.md", Side: review.SideNew,
				Line: 1, EndLine: 1, LineText: "hello"},
			out: "on k0 docs/a.md:1",
		},
		{
			name: "a range on a commit",
			args: []string{"--shortcode", "knl", "--file", "src/auth/token.go", "--line", "1", "--end-line", "3"},
			want: review.Anchor{Kind: review.KindCommit, ChangeID: "change-token", CommitID: "0123456789abcdef",
				Branch: "auth", Path: "src/auth/token.go", Side: review.SideNew, Line: 1, EndLine: 3,
				LineText: "package auth\n\n" + tokenLine},
			out: "on knl src/auth/token.go:1-3",
		},
		{
			name: "a commit by sha",
			args: []string{"--shortcode", "0123456", "--file", "src/auth/token.go", "--line", "3"},
			want: review.Anchor{Kind: review.KindCommit, ChangeID: "change-token", CommitID: "0123456789abcdef",
				Branch: "auth", Path: "src/auth/token.go", Side: review.SideNew, Line: 3, EndLine: 3, LineText: tokenLine},
			out: "on knl src/auth/token.go:3",
		},
		{
			// Both commits show line 3; the one that adds it wins over the newer one that has it as context.
			name: "a branch picks the commit that adds the line",
			args: []string{"--shortcode", "auth", "--file", "src/auth/token.go", "--line", "3"},
			want: review.Anchor{Kind: review.KindCommit, ChangeID: "change-token", CommitID: "0123456789abcdef",
				Branch: "auth", Path: "src/auth/token.go", Side: review.SideNew, Line: 3, EndLine: 3, LineText: tokenLine},
			out: "on knl src/auth/token.go:3",
		},
		{
			name: "a branch by cli id",
			args: []string{"--shortcode", "au", "--file", "src/auth/token.go", "--line", "4"},
			want: review.Anchor{Kind: review.KindCommit, ChangeID: "change-tidy", CommitID: "fedcba9876543210",
				Branch: "auth", Path: "src/auth/token.go", Side: review.SideNew, Line: 4, EndLine: 4,
				LineText: "// Token is for tests."},
			out: "on tdy src/auth/token.go:4",
		},
		{
			name: "a committed file's id",
			args: []string{"--shortcode", "k:m", "--line", "3"},
			want: review.Anchor{Kind: review.KindCommit, ChangeID: "change-token", CommitID: "0123456789abcdef",
				Branch: "auth", Path: "src/auth/token.go", Side: review.SideNew, Line: 3, EndLine: 3, LineText: tokenLine},
			out: "on knl src/auth/token.go:3",
		},
		{
			name: "an assigned file's id",
			args: []string{"--shortcode", "rd", "--file", "docs/a.md", "--line", "1"},
			want: review.Anchor{Kind: review.KindAssigned, Branch: "docs", Path: "docs/a.md", Side: review.SideNew,
				Line: 1, EndLine: 1, LineText: "hello"},
			out: "on k0 docs/a.md:1",
		},
		{
			name: "the old side",
			args: []string{"--shortcode", "zz", "--file", "README.md", "--line", "3", "--side", "old"},
			want: review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Side: review.SideOld, Line: 3, EndLine: 3,
				LineText: "A small service."},
			out: "on zz README.md:3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			c := h.comment(append(tc.args, "--body", "[nit] Look here")...)
			if c.Anchor != tc.want {
				t.Errorf("anchor\n got  %+v\n want %+v", c.Anchor, tc.want)
			}
			if c.Author != "agent" || c.Body != "[nit] Look here" || c.Status != review.StatusOpen {
				t.Errorf("comment %+v", c)
			}
			if !strings.Contains(h.stdout.String(), tc.out) {
				t.Errorf("printed %q, want %q", h.stdout.String(), tc.out)
			}
		})
	}
}

// An agent's comment reads back in the list like the user's, located where it was left.
func TestCommentListed(t *testing.T) {
	h := newHarness(t)
	mine := h.add(review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Line: 5, LineText: "## Usage"}, "Mine")
	theirs := h.comment("--file", "src/auth/token.go", "--line", "3", "--shortcode", "knl", "--body", "[must-fix] Secret")
	named := h.comment("--file", "README.md", "--line", "7", "--body", "[question] Why ./src?", "--author", "claude")

	got := decode(t, h.ok("list", "--json"))
	if len(got) != 3 || got[0]["author"] != "user" || got[1]["author"] != "agent" || got[2]["author"] != "claude" {
		t.Fatalf("list: %v", got)
	}
	if got[1]["shortcode"] != "knl" || got[1]["line"] != 3.0 || got[1]["outdated"] != false {
		t.Errorf("agent comment %v", got[1])
	}
	if got := decode(t, h.ok("list", "--author", "agent", "--json")); len(got) != 1 || got[0]["id"] != theirs.ID {
		t.Errorf("--author agent: %v", got)
	}
	if got := decode(t, h.ok("list", "--author", "user", "--json")); len(got) != 1 || got[0]["id"] != mine.ID {
		t.Errorf("--author user: %v", got)
	}
	if got := h.ok("list", "--author", "nobody"); got != "No open comments by nobody.\n" {
		t.Errorf("empty --author: %q", got)
	}
	if out := h.ok("show", named.ID); !strings.Contains(out, "by claude") {
		t.Errorf("show:\n%s", out)
	}
}

func TestCommentErrors(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		args []string
		code int
		msg  string
	}{
		{[]string{"--line", "1", "--body", "x"}, 2, "--file is required"},
		{[]string{"--file", "README.md", "--body", "x"}, 2, "--line is required"},
		{[]string{"--file", "README.md", "--line", "3", "--end-line", "2", "--body", "x"}, 2, "--end-line is before --line"},
		{[]string{"--file", "README.md", "--line", "1", "--side", "left", "--body", "x"}, 2, `invalid --side "left"`},
		{[]string{"--file", "README.md", "--line", "1"}, 2, "--body is required"},
		{[]string{"--file", "README.md", "--line", "1", "--body", "x", "extra"}, 2, `unexpected argument "extra"`},
		{[]string{"--file", "README.md", "--line", "9", "--body", "x"}, 1,
			"line 9 of README.md on the new side is not in the diff of zz"},
		{[]string{"--file", "README.md", "--line", "4", "--end-line", "9", "--body", "x"}, 1, "lines 4-9 of README.md"},
		{[]string{"--file", "src/auth/token.go", "--line", "1", "--body", "x"}, 1,
			"src/auth/token.go has no uncommitted changes; pass --shortcode"},
		{[]string{"--file", "docs/a.md", "--line", "1", "--shortcode", "zz", "--body", "x"}, 1,
			"zz (the unassigned changes) does not change docs/a.md"},
		{[]string{"--file", "README.md", "--line", "1", "--shortcode", "knl", "--body", "x"}, 1,
			"commit knl does not change README.md"},
		{[]string{"--file", "README.md", "--line", "1", "--shortcode", "auth", "--body", "x"}, 1,
			"no commit on branch auth changes README.md"},
		{[]string{"--file", "README.md", "--line", "1", "--shortcode", "nope", "--body", "x"}, 1,
			"no stack, commit or branch nope"},
		{[]string{"--file", "README.md", "--line", "1", "--shortcode", "k:m", "--body", "x"}, 1,
			"k:m is src/auth/token.go, not README.md"},
		{[]string{"--line", "1", "--shortcode", "knl", "--body", "x"}, 2, "--file is required unless"},
	} {
		code := h.run(append([]string{"comment"}, tc.args...)...)
		if code != tc.code || !strings.Contains(h.stderr.String(), tc.msg) {
			t.Errorf("%v: exit %d, stderr %q; want exit %d with %q", tc.args, code, h.stderr.String(), tc.code, tc.msg)
		}
	}
	if cs, _ := h.store.List(); len(cs) != 0 {
		t.Errorf("a failed comment was stored: %+v", cs)
	}
	if !NeedsBut([]string{"comment"}) {
		t.Error("comment reads the workspace but does not say so")
	}
}

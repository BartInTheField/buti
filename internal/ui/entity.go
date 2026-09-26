package ui

import (
	"github.com/bartinthefield/buti/internal/but"
)

type entityKind int

const (
	entNone          entityKind = iota
	entArea                     // every uncommitted change ("zz")
	entDir                      // directory in the unstaged tree: all files below it
	entFile                     // uncommitted file, unassigned or assigned to a stack
	entHunk                     // single uncommitted hunk
	entBranch                   // branch (a segment of a stack)
	entCommit                   // commit on an applied branch
	entCommittedFile            // file inside a commit
	entNewBranch                // the "new branch" drop lane, only a target
	entConflict                 // file of the commit in edit mode, with or without conflict markers
)

func (k entityKind) String() string {
	return [...]string{"nothing", "all changes", "directory", "file", "hunk", "branch", "commit", "committed file", "new branch", "conflicted file"}[k]
}

// uncommitted reports whether the entity is (a set of) uncommitted changes.
func (k entityKind) uncommitted() bool {
	return k == entArea || k == entDir || k == entFile || k == entHunk
}

// entity is anything that can be selected, marked, or be the source or target of an operation.
type entity struct {
	kind  entityKind
	id    string   // CLI id; "zz" for the area
	ids   []string // CLI ids of the files below, for directories
	label string   // human readable: path, branch name or commit subject

	branch string      // owning (or own) branch name, for branches, commits and committed files
	stack  string      // owning stack CLI id
	commit *but.Commit // for commits, and the parent commit of committed files
	status string      // branch status, for branches; "conflicted" or "resolved" for conflicted files
}

// cliIDs returns the ids to pass to `but` for this entity.
func (e entity) cliIDs() []string {
	if e.kind == entDir {
		return e.ids
	}
	if e.id == "" {
		return nil
	}
	return []string{e.id}
}

func (e entity) valid() bool { return e.kind != entNone }

// key identifies the entity across reloads, where CLI ids may shift.
func (e entity) key() string {
	switch e.kind {
	case entArea, entNewBranch:
		return e.kind.String()
	case entBranch:
		return "branch:" + e.branch
	case entCommit:
		return "commit:" + commitKey(e.commit)
	case entCommittedFile:
		return "cfile:" + commitKey(e.commit) + ":" + e.label
	case entHunk:
		return "hunk:" + e.id
	}
	return e.kind.String() + ":" + e.label
}

func commitKey(c *but.Commit) string {
	if c == nil {
		return ""
	}
	if c.ChangeID != "" {
		return c.ChangeID
	}
	return c.CommitID
}

// markClass groups kinds that may be marked together: `but` accepts files and hunks
// in one call, but never mixes changes, commits, committed files and branches.
func (k entityKind) markClass() int {
	switch k {
	case entDir, entFile, entHunk:
		return 1
	case entCommit:
		return 2
	case entCommittedFile:
		return 3
	case entBranch:
		return 4
	}
	return 0
}

// describe is a short phrase for status messages: `commit "fix typo"`, `branch feat-x`.
func (e entity) describe() string {
	switch e.kind {
	case entArea:
		return "all uncommitted changes"
	case entCommit:
		return "commit “" + e.label + "”"
	case entBranch:
		return "branch " + e.label
	case entNone:
		return "nothing"
	}
	return e.kind.String() + " " + e.label
}

// describeAll describes a set of entities of the same kind.
func describeAll(es []entity) string {
	if len(es) == 1 {
		return es[0].describe()
	}
	return itoa(len(es)) + " " + es[0].kind.String() + "s"
}

// kinds reports whether every entity is one of the given kinds.
func allOf(es []entity, kinds ...entityKind) bool {
	for _, e := range es {
		ok := false
		for _, k := range kinds {
			ok = ok || e.kind == k
		}
		if !ok {
			return false
		}
	}
	return len(es) > 0
}

func collectIDs(es []entity) []string {
	var ids []string
	for _, e := range es {
		ids = append(ids, e.cliIDs()...)
	}
	return ids
}

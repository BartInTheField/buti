package desktop

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestOpenAndFile(t *testing.T) {
	srv := start(t, writeBut(t, "#!/bin/sh\nexit 1\n"))
	dir := srv.client.Dir
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("<<<<<<< ours\na\n=======\nb\n>>>>>>> theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var opened []string
	var inEditor bool
	orig := openFiles
	openFiles = func(editor bool, paths []string) error {
		inEditor, opened = editor, paths
		return nil
	}
	t.Cleanup(func() { openFiles = orig })

	if res := post(t, srv, "/open", "", `{"paths":["CHANGELOG.md"]}`, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("open without token: %d", res.StatusCode)
	}
	res := post(t, srv, "/open", srv.Token, `{"paths":["CHANGELOG.md"],"editor":true}`, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("open %d: %v", res.StatusCode, decode(t, res))
	}
	if want := []string{filepath.Join(dir, "CHANGELOG.md")}; !inEditor || !slices.Equal(opened, want) {
		t.Fatalf("opened %v (editor %v), want %v", opened, inEditor, want)
	}
	for _, body := range []string{`{"paths":[]}`, `{"paths":["../x"]}`, `{"paths":["/etc/hosts"]}`, `{"paths":["missing.md"]}`} {
		if res := post(t, srv, "/open", srv.Token, body, ""); res.StatusCode != http.StatusBadRequest {
			t.Errorf("open %s: %d", body, res.StatusCode)
		}
	}

	res = get(t, srv, "/file?path=CHANGELOG.md", srv.Token, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("file %d", res.StatusCode)
	}
	if body := decode(t, res); body["content"] != "<<<<<<< ours\na\n=======\nb\n>>>>>>> theirs\n" {
		t.Fatalf("file body %#v", body)
	}
	if res := get(t, srv, "/file?path=../secret", srv.Token, ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("file outside the repo: %d", res.StatusCode)
	}
	if res := get(t, srv, "/file?path=nope.md", srv.Token, ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("missing file: %d", res.StatusCode)
	}
	if res := get(t, srv, "/file?path=CHANGELOG.md", "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("file without token: %d", res.StatusCode)
	}

	// A symlink pointing out of the repository is refused.
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	if res := get(t, srv, "/file?path=out", srv.Token, ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("symlink out of the repo: %d", res.StatusCode)
	}
}

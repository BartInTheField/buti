// Package update checks GitHub for a newer buti release and installs it over the running binary.
package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// BaseURL is the repository's GitHub URL; tests point it at a fake server.
var BaseURL = "https://github.com/BartInTheField/buti"

// cacheTTL is how long a looked-up latest version is trusted before asking GitHub again.
const cacheTTL = 24 * time.Hour

var client = &http.Client{
	Timeout: 30 * time.Second,
	// Keep the redirect of /releases/latest so its Location names the version.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Latest returns the newest published version, reusing the answer cached in cacheFile for a day.
// An empty cacheFile disables the cache.
func Latest(ctx context.Context, cacheFile string) (string, error) {
	if cacheFile != "" {
		if fi, err := os.Stat(cacheFile); err == nil && time.Since(fi.ModTime()) < cacheTTL {
			if b, err := os.ReadFile(cacheFile); err == nil && len(b) > 0 {
				return strings.TrimSpace(string(b)), nil
			}
		}
	}
	v, err := fetchLatest(ctx)
	if err != nil {
		return "", err
	}
	if cacheFile != "" {
		_ = os.MkdirAll(filepath.Dir(cacheFile), 0o755)
		_ = os.WriteFile(cacheFile, []byte(v+"\n"), 0o644)
	}
	return v, nil
}

func fetchLatest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, BaseURL+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/releases/tag/") {
		return "", errors.New("no release published yet")
	}
	return path.Base(loc), nil
}

// Newer reports whether version a (YYYY.MM.DD.N) is newer than b. Anything unparsable is never newer.
func Newer(a, b string) bool {
	pa, oka := parse(a)
	pb, okb := parse(b)
	if !oka || !okb {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

// IsRelease reports whether v is a release version, as opposed to a development build.
func IsRelease(v string) bool {
	_, ok := parse(v)
	return ok
}

func parse(v string) ([4]int, bool) {
	var out [4]int
	parts := strings.Split(v, ".")
	if len(parts) != 4 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Install downloads version for this platform, verifies its checksum and replaces the binary at exe.
func Install(ctx context.Context, version, exe string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("self-update is not supported on Windows; download it from %s/releases/latest", BaseURL)
	}
	name := fmt.Sprintf("buti_%s_%s_%s", version, runtime.GOOS, runtime.GOARCH)
	base := BaseURL + "/releases/download/" + version

	sums, err := download(ctx, base+"/checksums.txt")
	if err != nil {
		return err
	}
	want := checksum(sums, name+".tar.gz")
	if want == "" {
		return fmt.Errorf("no checksum for %s.tar.gz", name)
	}
	archive, err := download(ctx, base+"/"+name+".tar.gz")
	if err != nil {
		return err
	}
	if got := sha256.Sum256(archive); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s.tar.gz", name)
	}
	bin, err := extract(archive, name+"/buti")
	if err != nil {
		return err
	}

	// Write next to exe and rename over it, so a failed update never leaves a broken binary.
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".buti-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), exe)
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Release assets redirect to a CDN, so follow redirects here.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// checksum finds file's hash in sha256sum output.
func checksum(sums []byte, file string) string {
	sc := bufio.NewScanner(strings.NewReader(string(sums)))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 && f[1] == file {
			return f[0]
		}
	}
	return ""
}

func extract(archive []byte, file string) ([]byte, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s not found in archive", file)
		}
		if err != nil {
			return nil, err
		}
		if h.Name == file {
			return io.ReadAll(tr)
		}
	}
}

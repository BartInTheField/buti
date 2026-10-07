package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
)

func TestFramingRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	msgs := []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		{"jsonrpc": "2.0", "method": "note", "params": map[string]any{"text": "héllo ↳ wörld"}},
	}
	for _, m := range msgs {
		if err := writeMessage(&buf, m); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.HasPrefix(buf.String(), "Content-Length: ") {
		t.Fatalf("no header: %q", buf.String())
	}
	r := bufio.NewReader(&buf)
	for _, want := range msgs {
		body, err := readMessage(r)
		if err != nil {
			t.Fatal(err)
		}
		wantBody, _ := json.Marshal(want)
		if string(body) != string(wantBody) {
			t.Fatalf("got %s, want %s", body, wantBody)
		}
	}
	if _, err := readMessage(r); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestReadMessageExtraHeaders(t *testing.T) {
	in := "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\ncontent-length: 2\r\n\r\n{}"
	body, err := readMessage(bufio.NewReader(strings.NewReader(in)))
	if err != nil || string(body) != "{}" {
		t.Fatalf("got %q, %v", body, err)
	}
	if _, err := readMessage(bufio.NewReader(strings.NewReader("Content-Length: x\r\n\r\n"))); err == nil {
		t.Fatal("want an error for a bad length")
	}
}

func TestURIRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix paths")
	}
	for _, p := range []string{"/tmp/a b/c#d%e.go", "/Users/me/ünï/x.go"} {
		uri := pathToURI(p)
		if strings.ContainsAny(uri, " #") {
			t.Fatalf("%q not encoded: %s", p, uri)
		}
		if got := uriToPath(uri); got != p {
			t.Fatalf("%s: got %q, want %q", uri, got, p)
		}
	}
	if got := uriToPath("file:///tmp/a%20b/x.go"); got != "/tmp/a b/x.go" {
		t.Fatalf("got %q", got)
	}
	if got := uriToPath("untitled:Untitled-1"); got != "" {
		t.Fatalf("got %q", got)
	}
}

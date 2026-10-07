package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
)

// JSON-RPC error codes the server uses.
const (
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// message is any JSON-RPC message: a request has an id and a method, a notification only a method, a response an id
// and a result or an error.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("%s (%d)", e.Message, e.Code) }

// isRequest reports whether m expects a response. A null id counts as one, which the spec allows.
func (m *message) isRequest() bool {
	return len(m.ID) > 0 && m.Method != ""
}

// readMessage reads one Content-Length framed message body.
func readMessage(r *bufio.Reader) ([]byte, error) {
	header, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("reading header: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(header.Get("Content-Length")))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("bad Content-Length %q", header.Get("Content-Length"))
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	return body, nil
}

// writeMessage writes v as one Content-Length framed message.
func writeMessage(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

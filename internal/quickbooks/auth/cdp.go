package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type cdpMsg struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpErr         `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type cdpErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *cdpErr) Error() string {
	if e == nil {
		return "cdp error"
	}
	return e.Message
}

type cdpConn struct {
	ws      *websocket.Conn
	next    atomic.Int64
	mu      sync.Mutex
	writeMu sync.Mutex
	pending map[int64]chan cdpMsg
	events  chan cdpMsg
	closed  chan struct{}
}

func dialCDP(ctx context.Context, wsURL string) (*cdpConn, error) {
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, _, err := d.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp dial: %w", err)
	}
	c := &cdpConn{
		ws:      ws,
		pending: make(map[int64]chan cdpMsg),
		events:  make(chan cdpMsg, 64),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func (c *cdpConn) readLoop() {
	defer close(c.closed)
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			c.failAll(err)
			return
		}
		var msg cdpMsg
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		if msg.ID != 0 {
			c.mu.Lock()
			ch := c.pending[msg.ID]
			delete(c.pending, msg.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		if msg.Method != "" {
			select {
			case c.events <- msg:
			default:
			}
		}
	}
}

func (c *cdpConn) failAll(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- cdpMsg{ID: id, Error: &cdpErr{Message: err.Error()}}
		delete(c.pending, id)
	}
}

func (c *cdpConn) call(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	id := c.next.Add(1)
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		raw = []byte("{}")
	}
	msg := cdpMsg{ID: id, Method: method, Params: raw, SessionID: sessionID}
	ch := make(chan cdpMsg, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	// gorilla/websocket panics on concurrent writes. waitPausedMatch
	// continues Fetch.requestPaused from goroutines while the capture
	// loop still calls other CDP methods.
	c.writeMu.Lock()
	err = c.ws.WriteJSON(msg)
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, fmt.Errorf("cdp connection closed")
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

func (c *cdpConn) close() {
	_ = c.ws.Close()
}

type relayVersion struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func relayDebuggerURL(ctx context.Context, relayURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, relayURL+"/json/version", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("relay /json/version returned %s", resp.Status)
	}
	var v relayVersion
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	if v.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("relay /json/version has no webSocketDebuggerUrl")
	}
	return v.WebSocketDebuggerURL, nil
}

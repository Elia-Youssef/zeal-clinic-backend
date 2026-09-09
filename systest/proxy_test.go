//go:build systest

package systest

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// faultProxy sits between the clinic and its cloud peer: the clinic's PEER_URL
// names the proxy port. It relays HTTP/1.1 connections byte for byte and can
// refuse or drop them, block requests by path, hold a request body or a
// response, corrupt one request body byte, or hand connections to an in-test
// fake peer. It records the request lines and the sync stream events it relays.
type faultProxy struct {
	ln    net.Listener
	fake  *chanListener
	mu    sync.Mutex
	mode  proxyMode
	dest  string
	conns map[io.Closer]struct{}
	rules []*proxyRule
	log   []proxyEvent
}

type proxyMode int

const (
	modeForward proxyMode = iota
	modeRefuse
	modeFake
)

type proxyEvent struct {
	at     time.Time
	kind   string // "request" or "event"
	method string
	path   string // request target, or the event name ("ping" for a keep-alive)
}

func (e proxyEvent) String() string {
	if e.kind == "event" {
		return "event " + e.path
	}
	return e.method + " " + e.path
}

// proxyRule applies to requests whose method and path prefix match.
type proxyRule struct {
	method   string
	prefix   string
	once     bool
	used     bool
	block    bool
	holdBody int64 // forward this many body bytes, then hold (-1: off)
	corrupt  int64 // flip the body byte at this offset (-1: off)
	holdResp bool  // hold the response until released or dropped
	gate     *gate
}

func (r *proxyRule) matches(method, target string) bool {
	return (r.method == "" || r.method == method) && strings.HasPrefix(target, r.prefix)
}

// gate pauses traffic: reached closes when the traffic arrives at the hold
// point, and release or drop decide whether it continues.
type gate struct {
	reached  chan struct{}
	done     chan struct{}
	reachOne sync.Once
	doneOne  sync.Once
	dropped  bool
}

func newGate() *gate { return &gate{reached: make(chan struct{}), done: make(chan struct{})} }

func (g *gate) pause() bool {
	g.reachOne.Do(func() { close(g.reached) })
	<-g.done
	return !g.dropped
}

func (g *gate) release() { g.doneOne.Do(func() { close(g.done) }) }

func (g *gate) drop() {
	g.doneOne.Do(func() {
		g.dropped = true
		close(g.done)
	})
}

func (g *gate) wait(d time.Duration) bool {
	select {
	case <-g.reached:
		return true
	case <-time.After(d):
		return false
	}
}

// happened reports whether the traffic has reached the gate, without waiting.
func (g *gate) happened() bool {
	select {
	case <-g.reached:
		return true
	default:
		return false
	}
}

func startProxy(port int, dest string) (*faultProxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, fmt.Errorf("proxy listen on %d: %w", port, err)
	}
	p := &faultProxy{
		ln:    ln,
		fake:  newChanListener(ln.Addr()),
		dest:  dest,
		conns: map[io.Closer]struct{}{},
	}
	go p.accept()
	return p, nil
}

func (p *faultProxy) close() {
	_ = p.ln.Close()
	_ = p.fake.Close()
	p.dropAll()
}

func (p *faultProxy) accept() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		go p.serve(c)
	}
}

func (p *faultProxy) setMode(m proxyMode) {
	p.mu.Lock()
	p.mode = m
	p.mu.Unlock()
	p.dropAll()
}

func (p *faultProxy) retarget(dest string) {
	p.mu.Lock()
	p.dest = dest
	p.mu.Unlock()
	p.dropAll()
}

// dropAll closes every open connection, the sync event stream included.
func (p *faultProxy) dropAll() {
	p.mu.Lock()
	conns := make([]io.Closer, 0, len(p.conns))
	for c := range p.conns {
		conns = append(conns, c)
	}
	p.conns = map[io.Closer]struct{}{}
	p.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

func (p *faultProxy) addRule(r *proxyRule) *gate {
	if r.gate == nil {
		r.gate = newGate()
	}
	p.mu.Lock()
	p.rules = append(p.rules, r)
	p.mu.Unlock()
	return r.gate
}

func (p *faultProxy) clearRules() {
	p.mu.Lock()
	rules := p.rules
	p.rules = nil
	p.mu.Unlock()
	for _, r := range rules {
		r.gate.release()
	}
}

// blockPaths refuses requests whose path starts with one of the prefixes;
// everything else passes. Open connections are dropped.
func (p *faultProxy) blockPaths(prefixes ...string) {
	for _, pre := range prefixes {
		p.addRule(&proxyRule{prefix: pre, block: true, holdBody: -1, corrupt: -1})
	}
	p.dropAll()
}

func (p *faultProxy) holdRequestBody(method, prefix string, after int64) *gate {
	return p.addRule(&proxyRule{method: method, prefix: prefix, once: true, holdBody: after, corrupt: -1})
}

func (p *faultProxy) corruptRequestBody(method, prefix string, offset int64) *gate {
	return p.addRule(&proxyRule{method: method, prefix: prefix, once: true, holdBody: -1, corrupt: offset})
}

func (p *faultProxy) holdResponse(method, prefix string) *gate {
	return p.addRule(&proxyRule{method: method, prefix: prefix, once: true, holdBody: -1, corrupt: -1, holdResp: true})
}

func (p *faultProxy) match(method, target string) *proxyRule {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.rules {
		if r.once && r.used {
			continue
		}
		if r.matches(method, target) {
			r.used = true
			return r
		}
	}
	return nil
}

func (p *faultProxy) record(e proxyEvent) {
	e.at = time.Now()
	p.mu.Lock()
	p.log = append(p.log, e)
	p.mu.Unlock()
}

// mark returns a position in the log for since.
func (p *faultProxy) mark() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.log)
}

func (p *faultProxy) since(mark int) []proxyEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	if mark > len(p.log) {
		mark = len(p.log)
	}
	return append([]proxyEvent(nil), p.log[mark:]...)
}

func (p *faultProxy) track(c io.Closer) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conns == nil {
		return false
	}
	p.conns[c] = struct{}{}
	return true
}

func (p *faultProxy) untrack(c io.Closer) {
	p.mu.Lock()
	delete(p.conns, c)
	p.mu.Unlock()
}

func (p *faultProxy) serve(client net.Conn) {
	p.mu.Lock()
	mode, dest := p.mode, p.dest
	p.mu.Unlock()
	switch mode {
	case modeRefuse:
		_ = client.Close()
		return
	case modeFake:
		p.track(client)
		if !p.fake.push(client) {
			_ = client.Close()
		}
		return
	}
	server, err := net.DialTimeout("tcp", dest, 5*time.Second)
	if err != nil {
		_ = client.Close()
		return
	}
	pc := &proxyConn{p: p, client: client, server: server}
	p.track(pc)
	defer p.untrack(pc)
	done := make(chan struct{})
	go func() {
		pc.relayResponses()
		close(done)
	}()
	pc.relayRequests()
	<-done
	_ = pc.Close()
}

type proxyConn struct {
	p      *faultProxy
	client net.Conn
	server net.Conn

	mu       sync.Mutex
	respGate *gate
	stream   bool
	closed   bool
}

func (c *proxyConn) Close() error {
	c.mu.Lock()
	c.closed = true
	g := c.respGate
	c.mu.Unlock()
	if g != nil {
		g.drop()
	}
	_ = c.client.Close()
	return c.server.Close()
}

var errDropped = errors.New("dropped by the proxy")

// relayRequests reads request heads from the clinic, applies the rules and
// forwards each request. Bodies are framed by Content-Length; a chunked body
// switches the rest of the connection to a plain copy. Like a TCP path, the
// proxy passes a clean end on as a half-close and anything else as a reset.
func (c *proxyConn) relayRequests() {
	r := bufio.NewReaderSize(c.client, 64<<10)
	for {
		head, method, target, length, chunked, err := readRequestHead(r)
		if err != nil {
			c.endRequests(err)
			return
		}
		c.p.record(proxyEvent{kind: "request", method: method, path: target})
		if strings.HasPrefix(target, "/api/sync/events") {
			c.mu.Lock()
			c.stream = true
			c.mu.Unlock()
		}
		rule := c.p.match(method, target)
		if rule != nil && rule.block {
			_ = c.Close()
			return
		}
		if rule != nil && rule.holdResp {
			c.mu.Lock()
			c.respGate = rule.gate
			c.mu.Unlock()
		}
		if _, err := c.server.Write(head); err != nil {
			_ = c.Close()
			return
		}
		if chunked {
			_, err := io.Copy(c.server, r)
			c.endRequests(err)
			return
		}
		if err := c.copyBody(r, length, rule); err != nil {
			c.endRequests(err)
			return
		}
	}
}

func (c *proxyConn) copyBody(r *bufio.Reader, n int64, rule *proxyRule) error {
	holdAt, corruptAt := int64(-1), int64(-1)
	if rule != nil {
		holdAt, corruptAt = rule.holdBody, rule.corrupt
	}
	if holdAt > n {
		holdAt = n
	}
	buf := make([]byte, 32<<10)
	var pos int64
	held := false
	for {
		if !held && holdAt >= 0 && pos >= holdAt {
			held = true
			if !rule.gate.pause() {
				return errDropped
			}
		}
		if pos >= n {
			return nil
		}
		want := n - pos
		if want > int64(len(buf)) {
			want = int64(len(buf))
		}
		if !held && holdAt >= 0 && holdAt-pos < want {
			want = holdAt - pos
		}
		m, err := io.ReadFull(r, buf[:want])
		if corruptAt >= pos && corruptAt < pos+int64(m) {
			buf[corruptAt-pos] ^= 0xFF
			rule.gate.reachOne.Do(func() { close(rule.gate.reached) })
		}
		if m > 0 {
			if _, werr := c.server.Write(buf[:m]); werr != nil {
				return werr
			}
		}
		pos += int64(m)
		if err != nil {
			return err
		}
	}
}

// relayResponses copies the peer's answers back, holding them while a
// response gate is set, and records the events of the sync stream.
func (c *proxyConn) relayResponses() {
	buf := make([]byte, 32<<10)
	var scan eventScanner
	for {
		m, err := c.server.Read(buf)
		if m > 0 {
			c.mu.Lock()
			g, stream := c.respGate, c.stream
			c.mu.Unlock()
			if g != nil {
				if !g.pause() {
					_ = c.Close()
					return
				}
				c.mu.Lock()
				c.respGate = nil
				c.mu.Unlock()
			}
			if stream {
				for _, name := range scan.feed(buf[:m]) {
					c.p.record(proxyEvent{kind: "event", path: name})
				}
			}
			if _, werr := c.client.Write(buf[:m]); werr != nil {
				_ = c.Close()
				return
			}
		}
		if err != nil {
			if tc, ok := c.client.(*net.TCPConn); ok && errors.Is(err, io.EOF) {
				_ = tc.CloseWrite()
				return
			}
			_ = c.Close()
			return
		}
	}
}

// endRequests passes the end of the clinic's side on: a clean end becomes a
// half-close towards the peer, which can still answer; anything else resets both.
func (c *proxyConn) endRequests(err error) {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		if tc, ok := c.server.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
			return
		}
	}
	_ = c.Close()
}

func readRequestHead(r *bufio.Reader) (head []byte, method, target string, length int64, chunked bool, err error) {
	var b strings.Builder
	first := true
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, "", "", 0, false, err
		}
		b.WriteString(line)
		trimmed := strings.TrimRight(line, "\r\n")
		if first {
			parts := strings.SplitN(trimmed, " ", 3)
			if len(parts) != 3 {
				return nil, "", "", 0, false, fmt.Errorf("bad request line %q", trimmed)
			}
			method, target = parts[0], parts[1]
			first = false
			continue
		}
		if trimmed == "" {
			return []byte(b.String()), method, target, length, chunked, nil
		}
		name, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "content-length":
			length, _ = strconv.ParseInt(value, 10, 64)
		case "transfer-encoding":
			chunked = strings.Contains(strings.ToLower(value), "chunked")
		}
	}
}

// eventScanner picks "event: <name>" lines and keep-alive comments out of an
// event stream, whatever the chunk boundaries.
type eventScanner struct{ line []byte }

func (s *eventScanner) feed(b []byte) []string {
	var names []string
	for _, ch := range b {
		if ch != '\n' {
			if len(s.line) < 512 {
				s.line = append(s.line, ch)
			}
			continue
		}
		l := strings.TrimRight(string(s.line), "\r")
		s.line = s.line[:0]
		if name, ok := strings.CutPrefix(l, "event: "); ok {
			names = append(names, name)
		} else if l == ": ping" {
			names = append(names, "ping")
		}
	}
	return names
}

// chanListener hands connections accepted by the proxy to the fake peer's
// HTTP server.
type chanListener struct {
	addr  net.Addr
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newChanListener(addr net.Addr) *chanListener {
	return &chanListener{addr: addr, conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *chanListener) push(c net.Conn) bool {
	select {
	case l.conns <- c:
		return true
	case <-l.done:
		return false
	case <-time.After(5 * time.Second):
		return false
	}
}

func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *chanListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *chanListener) Addr() net.Addr { return l.addr }

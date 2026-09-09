//go:build systest

package systest

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	breakHelperEnv = "SYSTEST_BREAK_PID"
	// jailProxy is a closed local port: every outbound HTTP call of a node fails
	// at once, while loopback traffic never goes through a proxy.
	jailProxy = "http://127.0.0.1:9"
)

// node is one server process: its own working folder (the database lives in
// ./tmp for the clinic and ./data for the cloud), its own LOCALAPPDATA and
// TEMP, and a console log per start.
type node struct {
	name    string
	bin     string
	port    int
	root    string
	dataDir string
	env     []string

	mu     sync.Mutex
	cmd    *exec.Cmd
	exited chan struct{}
	logs   []string
}

func newNode(name, bin string, port int, root, dataDir string) (*node, error) {
	if _, err := os.Stat(root); err == nil {
		return nil, fmt.Errorf("%s already exists: the suite needs a fresh work folder", root)
	}
	local := filepath.Join(root, "localappdata")
	tmp := filepath.Join(root, "tmp-env")
	for _, d := range []string{local, tmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return &node{name: name, bin: bin, port: port, root: root, dataDir: dataDir, env: jailedEnv(local, tmp)}, nil
}

func jailedEnv(local, tmp string) []string {
	drop := map[string]bool{
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "LOCALAPPDATA": true,
		"TEMP": true, "TMP": true, "TMPDIR": true, "GOFLAGS": true, "GOPROXY": true,
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if drop[strings.ToUpper(k)] {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HTTP_PROXY="+jailProxy, "HTTPS_PROXY="+jailProxy, "LOCALAPPDATA="+local, "TEMP="+tmp, "TMP="+tmp)
	if runtime.GOOS != "windows" {
		env = append(env, "http_proxy="+jailProxy, "https_proxy="+jailProxy, "TMPDIR="+tmp)
	}
	return env
}

func (n *node) url(path string) string {
	return "http://127.0.0.1:" + strconv.Itoa(n.port) + path
}

func (n *node) addr() string { return "127.0.0.1:" + strconv.Itoa(n.port) }

func (n *node) dbPath() string { return filepath.Join(n.root, n.dataDir, "clinic.db") }

func (n *node) backupDir() string { return filepath.Join(n.root, "backup") }

// runOnce runs the binary to completion, for the seed step.
func (n *node) runOnce(t *testing.T, logName string, args ...string) {
	t.Helper()
	logPath := filepath.Join(n.root, logName)
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(n.bin, args...)
	cmd.Dir = n.root
	cmd.Env = n.env
	cmd.Stdout = f
	cmd.Stderr = f
	err = startInGroup(cmd)
	f.Close()
	if err != nil {
		t.Fatalf("%s: start %v: %v", n.name, args, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(10 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatalf("%s %v did not finish within 10 minutes", n.name, args)
	}
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", n.name, args, err, tail(logPath, 20))
	}
}

// start runs the server with --dev and waits until /health answers.
func (n *node) start(t *testing.T) {
	t.Helper()
	if listening(n.addr()) {
		t.Fatalf("%s: port %d is already in use", n.name, n.port)
	}
	n.mu.Lock()
	logPath := filepath.Join(n.root, fmt.Sprintf("console-%d.log", len(n.logs)+1))
	n.mu.Unlock()
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(n.bin, "--dev")
	cmd.Dir = n.root
	cmd.Env = n.env
	cmd.Stdout = f
	cmd.Stderr = f
	err = startInGroup(cmd)
	f.Close()
	if err != nil {
		t.Fatalf("%s: start: %v", n.name, err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	n.mu.Lock()
	n.cmd, n.exited = cmd, exited
	n.logs = append(n.logs, logPath)
	n.mu.Unlock()

	deadline := time.Now().Add(60 * time.Second)
	for {
		select {
		case <-exited:
			t.Fatalf("%s exited during start (%s)\n%s", n.name, cmd.ProcessState, tail(logPath, 20))
		default:
		}
		if healthy(n) {
			return
		}
		if time.Now().After(deadline) {
			n.kill(t)
			t.Fatalf("%s: no healthy answer within 60 s\n%s", n.name, tail(logPath, 20))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func healthy(n *node) bool {
	c := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	resp, err := c.Get(n.url("/health"))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var body struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode == http.StatusOK && body.Status == "ok"
}

// stop is the graceful stop: Ctrl+Break, then the server must exit on its own
// with code 0 after logging its shutdown.
func (n *node) stop(t *testing.T) {
	t.Helper()
	n.mu.Lock()
	cmd, exited := n.cmd, n.exited
	logPath := ""
	if len(n.logs) > 0 {
		logPath = n.logs[len(n.logs)-1]
	}
	n.mu.Unlock()
	if cmd == nil {
		return
	}
	if err := interrupt(cmd.Process.Pid); err != nil {
		n.kill(t)
		t.Fatalf("%s: Ctrl+Break not delivered (%v): stopped hard instead", n.name, err)
	}
	select {
	case <-exited:
	case <-time.After(30 * time.Second):
		n.kill(t)
		t.Fatalf("%s did not exit within 30 s of Ctrl+Break: stopped hard", n.name)
	}
	n.mu.Lock()
	n.cmd = nil
	n.mu.Unlock()
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("%s: exit code %d after Ctrl+Break", n.name, code)
	}
	if !strings.Contains(readFile(logPath), "Server shutting down") {
		t.Fatalf("%s: no shutdown message after Ctrl+Break\n%s", n.name, tail(logPath, 10))
	}
}

// kill ends the process at once, like a crash or a power cut.
func (n *node) kill(t *testing.T) {
	t.Helper()
	n.mu.Lock()
	cmd, exited := n.cmd, n.exited
	n.cmd = nil
	n.mu.Unlock()
	if cmd == nil {
		return
	}
	_ = cmd.Process.Kill()
	select {
	case <-exited:
	case <-time.After(15 * time.Second):
		t.Fatalf("%s: still running 15 s after a hard stop", n.name)
	}
}

// abort ends the process without checks, for cleanup after a failure.
func (n *node) abort() {
	n.mu.Lock()
	cmd, exited := n.cmd, n.exited
	n.cmd = nil
	n.mu.Unlock()
	if cmd == nil {
		return
	}
	_ = cmd.Process.Kill()
	select {
	case <-exited:
	case <-time.After(15 * time.Second):
	}
}

// logText is the console output of every start so far.
func (n *node) logText() string {
	n.mu.Lock()
	logs := append([]string(nil), n.logs...)
	n.mu.Unlock()
	var b strings.Builder
	for _, l := range logs {
		b.WriteString(readFile(l))
	}
	return b.String()
}

// currentLog is the console output of the running process.
func (n *node) currentLog() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.logs) == 0 {
		return ""
	}
	return readFile(n.logs[len(n.logs)-1])
}

func readFile(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func tail(path string, lines int) string {
	all := strings.Split(strings.TrimRight(readFile(path), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

func listening(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// logLines returns the lines of text that contain every needle.
func logLines(text string, needles ...string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		all := true
		for _, n := range needles {
			if !strings.Contains(l, n) {
				all = false
				break
			}
		}
		if all {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

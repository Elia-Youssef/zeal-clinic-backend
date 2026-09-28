//go:build systest

package systest

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func stepBackupDrill(t *testing.T, h *harness) {
	// The first marker exists before the restart, so the snapshot taken after it has it.
	h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/allergies", map[string]any{"name": "Systest markeralpha"})
	eventually(t, converge, "first marker on the clinic", func() (bool, string) {
		n, err := h.clinicAdmin.totalOrError("/api/allergies", "markeralpha")
		return err == nil && n == 1, fmt.Sprint(n, err)
	})
	h.cloud.stop(t)
	restarted := time.Now()
	h.cloud.start(t)

	// The monitor's first minute tick after a start takes a snapshot.
	backup := ""
	eventually(t, 75*time.Second, "first backup after the restart", func() (bool, string) {
		lines := logLines(h.cloud.currentLog(), "monitor: db backup -> ")
		if len(lines) == 0 {
			return false, "no backup yet"
		}
		_, path, _ := strings.Cut(lines[0], "monitor: db backup -> ")
		backup = strings.TrimSpace(path)
		if !filepath.IsAbs(backup) {
			backup = filepath.Join(h.cloud.root, backup)
		}
		return true, ""
	})
	t.Logf("snapshot %s, %s after the restart", filepath.Base(backup), time.Since(restarted).Round(time.Second))
	if filepath.Dir(backup) != h.cloud.backupDir() || !strings.HasPrefix(filepath.Base(backup), "clinic-") {
		t.Fatalf("snapshot path %s, want clinic-*.db in %s", backup, h.cloud.backupDir())
	}

	// The second marker comes after the snapshot and never reaches the clinic.
	h.disconnect()
	h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/allergies", map[string]any{"name": "Systest markerbravo"})
	h.cloud.stop(t)
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(h.cloud.dbPath() + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if err := copyFile(h.cloud.dbPath(), backup); err != nil {
		t.Fatal(err)
	}
	h.cloud.start(t)
	if n := h.cloudAdmin.total(t, "/api/allergies", "markeralpha"); n != 1 {
		t.Fatalf("first marker after the restore: %d, want 1", n)
	}
	if n := h.cloudAdmin.total(t, "/api/allergies", "markerbravo"); n != 0 {
		t.Fatalf("second marker after the restore: %d, want 0", n)
	}
	h.cloudAdmin.expect(t, http.StatusOK, http.MethodGet, "/api/auth/verify", nil)

	// Sync carries on in both directions.
	h.reconnect()
	p := h.createPatient(t, h.clinicAdmin, "Systest", "Restored")
	h.cloudAdmin.waitStatus(t, "/api/patients/"+p.ID, http.StatusOK)
	h.waitGateOpen(t, streamReconnect)
	h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/allergies", map[string]any{"name": "Systest markercharlie"})
	eventually(t, converge, "cloud write on the clinic after the restore", func() (bool, string) {
		n, err := h.clinicAdmin.totalOrError("/api/allergies", "markercharlie")
		return err == nil && n == 1, fmt.Sprint(n, err)
	})

	// A clinic running with --dev takes no snapshots, although it has seen
	// several minute ticks by now.
	if list := backups(t, h.clinic.backupDir()); len(list) != 0 {
		t.Fatalf("the --dev clinic made snapshots: %v", list)
	}
}

func backups(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "clinic-") && strings.HasSuffix(e.Name(), ".db") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// pushDrift writes an allergy straight into the cloud's sync API. Applied rows
// are never logged for the peer, so the clinic never pulls it: a row only the
// cloud has, as after a manual edit or a lost update.
func (h *harness) pushDrift(t *testing.T, name string) string {
	t.Helper()
	id := newID()
	now := time.Now().UTC().Format(time.RFC3339)
	row, _ := json.Marshal(map[string]any{"id": id, "name": name, "description": "", "created_at": now})
	body, _ := json.Marshal(map[string]any{"rows": []syncRow{{Seq: 1, Table: "allergies", RowID: id, Op: "insert", RowJSON: row, CreatedAt: now}}})
	if r := h.machine(t, h.cloud, http.MethodPost, "/api/sync/push", nil, body); r.status != http.StatusOK {
		t.Fatalf("drift push: %s", r)
	}
	if n := h.cloudAdmin.total(t, "/api/allergies", name); n != 1 {
		t.Fatalf("drift row %q on the cloud: %d", name, n)
	}
	return id
}

func stepCloudRestoreFailures(t *testing.T, h *harness) {
	h.pushDrift(t, "Systest driftone")
	before := backups(t, h.cloud.backupDir())

	// A corrupted upload fails its checksum on the cloud before the cloud takes
	// its own backup, so there is nothing to roll back.
	g := h.proxy.corruptRequestBody(http.MethodPost, "/api/cloud-restore", 4096)
	r := h.clinicAdmin.call(t, http.MethodPost, "/api/cloud-restore", nil)
	if r.status != http.StatusBadGateway || !strings.Contains(r.env.Error, "checksum mismatch") {
		t.Fatalf("restore with a corrupted upload: %s, want 502 checksum mismatch", r)
	}
	if !g.happened() {
		t.Fatal("the proxy corrupted nothing")
	}
	if len(logLines(h.cloud.currentLog(), "| 500 |", "POST /api/cloud-restore")) == 0 {
		t.Fatal("the cloud did not answer the corrupted upload with 500")
	}
	h.checkCloudUntouched(t, before, "Systest driftone")

	// An upload that is no database is refused after its checksum, also before
	// any backup.
	body := make([]byte, 64<<10)
	_, _ = rand.Read(body)
	sum := sha256.Sum256(body)
	r = h.machine(t, h.cloud, http.MethodPost, "/api/cloud-restore", map[string]string{
		"Content-Type": "application/octet-stream", "X-Restore-SHA256": hex.EncodeToString(sum[:]), "X-Restore-ID": "systest-not-a-database",
	}, body)
	if r.status != http.StatusInternalServerError || !strings.Contains(r.env.Error, "uploaded dump") {
		t.Fatalf("restore of a file that is no database: %s", r)
	}
	t.Logf("not a database: %s", r.env.Error)
	h.checkCloudUntouched(t, before, "Systest driftone")
}

// checkCloudUntouched: no new backup, the drift row still there, and both
// nodes out of maintenance.
func (h *harness) checkCloudUntouched(t *testing.T, before []string, drift string) {
	t.Helper()
	if after := backups(t, h.cloud.backupDir()); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Fatalf("cloud backups changed: %v -> %v", before, after)
	}
	if n := h.cloudAdmin.total(t, "/api/allergies", drift); n != 1 {
		t.Fatalf("drift row %q on the cloud: %d, want 1", drift, n)
	}
	h.clinicAdmin.expect(t, http.StatusOK, http.MethodGet, "/api/patients?limit=1&filter=", nil)
}

type progress struct {
	Status string `json:"status"`
	Stage  string `json:"stage"`
	Step   int    `json:"step"`
}

func stepCloudRestore(t *testing.T, h *harness) {
	h.pushDrift(t, "Systest drifttwo")
	// A user only the cloud knows, with a token and a notification. Sync paths
	// are blocked so the clinic never pulls it; the restore path stays open.
	h.proxy.blockPaths("/api/sync/")
	password := "St-" + randomText(12)
	h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/users", map[string]any{
		"username": "systest-cloud-only", "password": password, "displayName": "Cloud Only", "role": "staff",
	})
	cloudOnly := h.login(t, h.cloud, "systest-cloud-only", password)
	cloudOnly.expect(t, http.StatusOK, http.MethodPost, "/api/notifications/test", nil)
	r := h.cloudAdmin.expect(t, http.StatusOK, http.MethodPost, "/api/notifications/test", nil)
	var note struct {
		ID string `json:"id"`
	}
	r.into(t, &note)
	before := backups(t, h.cloud.backupDir())

	clinicEvents := h.clinicAdmin.watch(t)
	cloudEvents := h.cloudAdmin.watch(t)

	// Hold the upload after 64 KiB: both nodes stay in maintenance meanwhile.
	hold := h.proxy.holdRequestBody(http.MethodPost, "/api/cloud-restore", 64<<10)
	done := make(chan reply, 1)
	go func() {
		r, err := h.clinicAdmin.do(http.MethodPost, "/api/cloud-restore", nil)
		if err != nil {
			r = reply{body: []byte(err.Error())}
		}
		done <- r
	}()
	if !hold.wait(converge) {
		t.Fatal("the upload never reached the proxy")
	}
	for _, s := range []*session{h.clinicAdmin, h.cloudAdmin} {
		r := s.call(t, http.MethodGet, "/api/patients?limit=1&filter=", nil)
		if r.status != http.StatusServiceUnavailable || r.env.Error != "Cloud restore in progress, please wait" {
			hold.release()
			t.Fatalf("API on %s during the restore: %s, want 503 maintenance", s.n.name, r)
		}
		if !healthy(s.n) {
			hold.release()
			t.Fatalf("/health on %s during the restore", s.n.name)
		}
	}
	hold.release()
	var res reply
	select {
	case res = <-done:
	case <-time.After(2 * time.Minute):
		t.Fatal("the restore did not finish within 2 minutes")
	}
	if res.status != http.StatusOK || !res.env.Success {
		t.Fatalf("cloud restore: %s", res)
	}
	var result struct {
		Tables        int    `json:"tables"`
		Rows          int64  `json:"rows"`
		LocalBaseline int64  `json:"localBaseline"`
		CloudBaseline int64  `json:"cloudBaseline"`
		Backup        string `json:"backup"`
	}
	res.into(t, &result)
	t.Logf("restore: %d tables, %d rows, baselines local %d cloud %d", result.Tables, result.Rows, result.LocalBaseline, result.CloudBaseline)

	// The cloud's synced tables now match the clinic's snapshot.
	for _, drift := range []string{"Systest driftone", "Systest drifttwo"} {
		if n := h.cloudAdmin.total(t, "/api/allergies", drift); n != 0 {
			t.Fatalf("drift row %q survived the restore", drift)
		}
	}
	for _, l := range append([]string{"/api/users"}, parityLists...) {
		if a, b := h.clinicAdmin.total(t, l, ""), h.cloudAdmin.total(t, l, ""); a != b {
			t.Fatalf("%s after the restore: clinic %d, cloud %d", l, a, b)
		}
	}
	// Tokens and notifications of surviving users stay; the vanished user's go.
	h.cloudAdmin.expect(t, http.StatusOK, http.MethodGet, "/api/auth/verify", nil)
	found := false
	for _, n := range h.cloudAdmin.notifications(t) {
		found = found || n.ID == note.ID
	}
	if !found {
		t.Fatal("the admin's cloud notification did not survive the restore")
	}
	if st := cloudOnly.status(http.MethodGet, "/api/auth/verify", nil); st != http.StatusUnauthorized {
		t.Fatalf("token of the vanished user: %d, want 401", st)
	}
	b, _ := json.Marshal(map[string]string{"username": "systest-cloud-only", "password": password})
	if r, err := h.send(http.MethodPost, h.cloud.url("/api/auth/login"), nil, b); err != nil || r.status != http.StatusUnauthorized {
		t.Fatalf("sign in of the vanished user: %v %v, want 401", r, err)
	}

	// The cloud backed itself up first.
	after := backups(t, h.cloud.backupDir())
	if len(after) != len(before)+1 || !contains(after, result.Backup) {
		t.Fatalf("cloud backups %v -> %v, restore named %q", before, after, result.Backup)
	}
	// The cloud's outbox is empty and its positions sit at the two baselines.
	r = h.machine(t, h.cloud, http.MethodGet, "/api/sync/status", nil, nil)
	var st struct {
		MaxLogSeq     int64 `json:"max_log_seq"`
		LastPushedSeq int64 `json:"last_pushed_seq"`
		LastPulledSeq int64 `json:"last_pulled_seq"`
	}
	if err := json.Unmarshal(r.body, &st); err != nil || st.MaxLogSeq != 0 || st.LastPushedSeq != result.CloudBaseline || st.LastPulledSeq != result.LocalBaseline {
		t.Fatalf("cloud sync status after the restore: %s", r)
	}

	// Progress events: every step of each side, ending in success.
	clinicEvents.expectStages(t, "clinic", "preparing", "exporting", "uploading", "finalizing", "completed")
	cloudEvents.expectStages(t, "cloud", "receiving", "validating", "backing_up", "applying", "reopening", "completed")

	// Sync resumes in both directions.
	h.proxy.clearRules()
	h.proxy.dropAll()
	p := h.createPatient(t, h.clinicAdmin, "Systest", "After Restore")
	h.cloudAdmin.waitStatus(t, "/api/patients/"+p.ID, http.StatusOK)
	h.waitGateOpen(t, streamReconnect)
	h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/allergies", map[string]any{"name": "Systest markerdelta"})
	eventually(t, converge, "cloud write on the clinic after the restore", func() (bool, string) {
		n, err := h.clinicAdmin.totalOrError("/api/allergies", "markerdelta")
		return err == nil && n == 1, fmt.Sprint(n, err)
	})
}

// eventWatch reads a node's /api/events stream in the background.
type eventWatch struct {
	mu     sync.Mutex
	events []streamEvent
	pings  int
	ended  bool
	cancel context.CancelFunc
}

type streamEvent struct {
	name string
	data string
}

func (s *session) watch(t *testing.T) *eventWatch {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.n.url("/api/events"), nil)
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("event stream on %s: %v", s.n.name, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		t.Fatalf("event stream on %s: status %d", s.n.name, resp.StatusCode)
	}
	w := &eventWatch{cancel: cancel}
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
		name := ""
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				w.mu.Lock()
				w.events = append(w.events, streamEvent{name: name, data: strings.TrimPrefix(line, "data: ")})
				w.mu.Unlock()
			case line == ": ping":
				w.mu.Lock()
				w.pings++
				w.mu.Unlock()
			}
		}
		w.mu.Lock()
		w.ended = true
		w.mu.Unlock()
	}()
	t.Cleanup(cancel)
	return w
}

func (w *eventWatch) progress() []progress {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []progress
	for _, e := range w.events {
		if e.name != "cloud_restore_progress" {
			continue
		}
		var p progress
		if json.Unmarshal([]byte(e.data), &p) == nil {
			out = append(out, p)
		}
	}
	return out
}

func (w *eventWatch) expectStages(t *testing.T, side string, stages ...string) {
	t.Helper()
	describe := func(ps []progress) string {
		var parts []string
		for _, p := range ps {
			parts = append(parts, fmt.Sprintf("%d:%s:%s", p.Step, p.Stage, p.Status))
		}
		return strings.Join(parts, " ")
	}
	last := stages[len(stages)-1]
	eventually(t, converge, side+" restore progress", func() (bool, string) {
		ps := w.progress()
		return len(ps) > 0 && ps[len(ps)-1].Stage == last, describe(ps)
	})
	ps := w.progress()
	ok := len(ps) == len(stages)
	for i := 0; ok && i < len(ps); i++ {
		status := "running"
		if i == len(ps)-1 {
			status = "success"
		}
		ok = ps[i].Stage == stages[i] && ps[i].Step == i+1 && ps[i].Status == status
	}
	if !ok {
		t.Fatalf("%s progress: %s, want stages %v", side, describe(ps), stages)
	}
	t.Logf("%s progress: %s", side, describe(ps))
}

func stepLongSSE(t *testing.T, h *harness) {
	if !h.cfg.full {
		t.Skip("full mode only")
	}
	// Both event streams stay open for 90 s and more, with a keep-alive every 25 s.
	w := h.clinicAdmin.watch(t)
	mark := h.proxy.mark()
	time.Sleep(95 * time.Second)
	w.mu.Lock()
	pings, ended := w.pings, w.ended
	w.mu.Unlock()
	if ended || pings < 3 {
		t.Fatalf("clinic event stream: %d pings, ended %v", pings, ended)
	}
	events := h.proxy.since(mark)
	if n := len(eventNames(events, "ping")); n < 3 {
		t.Fatalf("sync event stream: %d pings in 95 s", n)
	}
	if n := len(requests(events, http.MethodGet, "/api/sync/events")); n != 0 {
		t.Fatalf("the sync event stream reconnected %d times", n)
	}
}

// stepLargeUpload restores a clinic database of about 50 MiB into the cloud:
// the restore upload has to pass whatever body limit the API applies elsewhere.
func stepLargeUpload(t *testing.T, h *harness) {
	if !h.cfg.full {
		t.Skip("full mode only")
	}
	const (
		targetBytes = int64(52 << 20)
		minUpload   = int64(20 << 20)
		noteBytes   = 512 << 10
		maxRows     = 400
	)
	// Patients with large notes grow the clinic database while the peer is out
	// of reach, so nothing is pushed row by row: the restore carries it all and
	// resets the outbox.
	h.disconnect()
	filler := strings.Repeat("Systest large upload filler text. ", noteBytes/34+1)[:noteBytes]
	before := h.clinicAdmin.total(t, "/api/patients", "")
	onDisk := func() int64 { return fileSize(h.clinic.dbPath()) + fileSize(h.clinic.dbPath()+"-wal") }
	rows := 0
	for rows < maxRows && onDisk() < targetBytes {
		h.phone++
		h.clinicAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/patients", map[string]any{
			"firstName": "Systest", "lastName": fmt.Sprintf("Bulk%03d", rows), "gender": "Female",
			"contact": strconv.Itoa(h.phone), "notes": fmt.Sprintf("%04d ", rows) + filler,
		})
		rows++
	}
	t.Logf("%d patients with %d KiB of notes each; clinic database %.1f MiB on disk", rows, noteBytes>>10, float64(onDisk())/(1<<20))

	h.reconnect()
	mark := h.proxy.mark()
	start := time.Now()
	r, err := h.clinicAdmin.doWithin(10*time.Minute, http.MethodPost, "/api/cloud-restore", nil)
	if err != nil {
		t.Fatalf("large cloud restore: %v", err)
	}
	if r.status != http.StatusOK || !r.env.Success {
		t.Fatalf("large cloud restore: %s", r)
	}
	elapsed := time.Since(start)
	uploads := requests(h.proxy.since(mark), http.MethodPost, "/api/cloud-restore")
	if len(uploads) != 1 {
		t.Fatalf("restore uploads through the proxy: %d, want 1", len(uploads))
	}
	size := uploads[0].length
	t.Logf("snapshot of %.1f MiB uploaded, applied and acknowledged in %s", float64(size)/(1<<20), elapsed.Round(100*time.Millisecond))
	if size < minUpload {
		t.Fatalf("uploaded snapshot %.1f MiB, want at least %d MiB", float64(size)/(1<<20), minUpload>>20)
	}

	// The cloud holds every bulk row, and sync carries on afterwards.
	a, b := h.clinicAdmin.total(t, "/api/patients", ""), h.cloudAdmin.total(t, "/api/patients", "")
	if a < before+rows || b != a {
		t.Fatalf("patients after the restore: clinic %d, cloud %d, want at least %d on both", a, b, before+rows)
	}
	p := h.createPatient(t, h.clinicAdmin, "Systest", "After Large Restore")
	h.cloudAdmin.waitStatus(t, "/api/patients/"+p.ID, http.StatusOK)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func stepShutdown(t *testing.T, h *harness) {
	// Ctrl+Break stops both nodes cleanly: exit code 0 after the shutdown message.
	h.clinic.stop(t)
	h.cloud.stop(t)
}

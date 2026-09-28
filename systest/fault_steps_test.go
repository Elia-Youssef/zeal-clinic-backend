//go:build systest

package systest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

// mismatchPulls counts the clinic's failed cycles against a peer on another version.
func (h *harness) mismatchPulls() int {
	return len(logLines(h.clinic.logText(), "[error] [sync] pull: peer pull status 409"))
}

func (h *harness) waitMismatchPulls(t *testing.T, n int) {
	t.Helper()
	eventually(t, converge, fmt.Sprintf("%d failed pulls against the other version", n), func() (bool, string) {
		got := h.mismatchPulls()
		return got >= n, fmt.Sprint(got)
	})
}

func stepVersionMismatch(t *testing.T, h *harness) {
	if h.alt == nil {
		t.Skip("no cloud build with another version stamp")
	}
	h.alt.start(t)
	altSuper := h.login(t, h.alt, "super-admin", h.superPassword)

	// Status stays open across versions; pull, push and the event stream refuse
	// another version with 409.
	if r := h.machine(t, h.alt, http.MethodGet, "/api/sync/status", nil, nil); r.status != http.StatusOK {
		t.Fatalf("sync status across versions: %s", r)
	}
	r := h.machine(t, h.alt, http.MethodGet, "/api/sync/pull?since=0", nil, nil)
	var mismatch struct {
		Error string `json:"error"`
		Peer  string `json:"peer"`
		Cloud string `json:"cloud"`
	}
	_ = json.Unmarshal(r.body, &mismatch)
	if r.status != http.StatusConflict || mismatch.Error != "version mismatch" || mismatch.Peer != "dev" || mismatch.Cloud != h.cfg.altVersion {
		t.Fatalf("pull from another version: %s", r)
	}

	h.clinicAdmin.expect(t, http.StatusOK, http.MethodPut, "/api/notifications/read-all", nil)
	if n := unreadSyncFailures(h.clinicAdmin.notifications(t)); len(n) != 0 {
		t.Fatalf("unread sync failure notifications before the episode: %d", len(n))
	}
	base := h.mismatchPulls()

	// The clinic's peer now runs another version: its cycles fail with 409 and
	// the first failure notifies every user once.
	h.proxy.retarget(h.alt.addr())
	h.createPatient(t, h.clinicAdmin, "Systest", "Mismatch A")
	h.waitMismatchPulls(t, base+1)
	var first notification
	eventually(t, converge, "one sync failure notification", func() (bool, string) {
		n := unreadSyncFailures(h.clinicAdmin.notifications(t))
		if len(n) == 1 {
			first = n[0]
		}
		return len(n) == 1, fmt.Sprint(len(n))
	})
	if first.Title != "Data sync failed" {
		t.Fatalf("notification title %q", first.Title)
	}

	// Read or not, further failures in the same episode add nothing. Cycles run
	// one at a time, so after the third failure the second one is complete.
	h.clinicAdmin.expect(t, http.StatusOK, http.MethodPut, "/api/notifications/"+first.ID+"/read", nil)
	h.createPatient(t, h.clinicAdmin, "Systest", "Mismatch B")
	h.waitMismatchPulls(t, base+2)
	h.createPatient(t, h.clinicAdmin, "Systest", "Mismatch C")
	h.waitMismatchPulls(t, base+3)
	if n := unreadSyncFailures(h.clinicAdmin.notifications(t)); len(n) != 0 {
		t.Fatalf("new sync failure notifications within one episode: %d", len(n))
	}

	// The other version never opens its gate, and a cloud restore to it is refused.
	if st, code := altSuper.gateStatus(); st != http.StatusServiceUnavailable || code != "sync_not_ready" {
		t.Fatalf("gate on the other version: %d %s", st, code)
	}
	r = h.clinicAdmin.call(t, http.MethodPost, "/api/cloud-restore", nil)
	if r.status != http.StatusBadGateway || !strings.Contains(r.env.Error, "Version mismatch") {
		t.Fatalf("cloud restore to another version: %s, want 502 Version mismatch", r)
	}
	if n := altSuper.total(t, "/api/patients", ""); n != 0 {
		t.Fatalf("the other version received %d patients", n)
	}

	// A good cycle ends the episode: back on the right version the rows written
	// meanwhile go out. The clinic ends the episode once a cycle has the answer
	// to its ready, or to its push when the stream has no session yet (it sends
	// no ready then). The retarget below cuts whatever is still on its way, so
	// the step waits for a ready after the push, which comes in both cases (at
	// the latest from the reconnected stream's first cycle), before the next
	// row. Cycles run one at a time: that row goes out in a later cycle, so once
	// it is on the cloud the cycle that ended the episode is over.
	mark := h.proxy.mark()
	h.proxy.retarget(h.cloud.addr())
	eventually(t, streamReconnect, "the clinic's push and ready on the right version", func() (bool, string) {
		events := h.proxy.since(mark)
		pushes := requests(events, http.MethodPost, "/api/sync/push")
		if len(pushes) == 0 {
			return false, "no push yet"
		}
		ready := requests(after(events, pushes[0].at), http.MethodPost, "/api/sync/ready")
		return len(ready) > 0, "no ready after the push yet"
	})
	h.createPatient(t, h.clinicAdmin, "Systest", "Mismatch D")
	eventually(t, converge, "mismatch patients on the cloud", func() (bool, string) {
		n, err := h.cloudAdmin.totalOrError("/api/patients", "Mismatch")
		return err == nil && n == 4, fmt.Sprint(n, err)
	})

	// The next failure is a new episode with a new notification.
	h.proxy.retarget(h.alt.addr())
	failed := h.mismatchPulls()
	h.createPatient(t, h.clinicAdmin, "Systest", "Mismatch E")
	h.waitMismatchPulls(t, failed+1)
	eventually(t, converge, "a new sync failure notification", func() (bool, string) {
		n := unreadSyncFailures(h.clinicAdmin.notifications(t))
		return len(n) == 1 && n[0].ID != first.ID, fmt.Sprint(len(n))
	})

	h.proxy.retarget(h.cloud.addr())
	h.createPatient(t, h.clinicAdmin, "Systest", "Mismatch F")
	eventually(t, converge, "all mismatch patients on the cloud", func() (bool, string) {
		n, err := h.cloudAdmin.totalOrError("/api/patients", "Mismatch")
		return err == nil && n == 6, fmt.Sprint(n, err)
	})
	// The event stream reconnects after its backoff (at most 30 s) and reopens the gate.
	h.waitGateOpen(t, streamReconnect)
	h.alt.stop(t)
}

func stepUpdatePeer(t *testing.T, h *harness) {
	if r := h.machine(t, h.cloud, http.MethodPost, "/api/update/peer", map[string]string{"X-Sync-Secret": ""}, nil); r.status != http.StatusUnauthorized {
		t.Fatalf("peer update without the secret: %s", r)
	}
	// Both nodes run the same build and nothing newer is published.
	if r := h.machine(t, h.cloud, http.MethodPost, "/api/update/peer", nil, nil); r.status != http.StatusConflict || r.env.Error != "No update available" {
		t.Fatalf("peer update on the same version: %s", r)
	}
	r := h.machine(t, h.cloud, http.MethodPost, "/api/update/peer", map[string]string{"X-Sync-Version": "0.0.0-other"}, nil)
	var d struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(r.env.Data, &d)
	if r.status != http.StatusAccepted || d.Status != "no update needed" {
		t.Fatalf("peer update from another version: %s", r)
	}
	if h.cfg.publishSecret == "" {
		return
	}
	if r, err := h.send(http.MethodPost, h.cloud.url("/api/versions"), nil, []byte("{}")); err != nil || r.status != http.StatusUnauthorized {
		t.Fatalf("publish without the secret: %v %v", r, err)
	}
	if r, err := h.send(http.MethodPost, h.cloud.url("/api/versions"), map[string]string{"X-Publish-Secret": h.cfg.publishSecret}, []byte("{}")); err != nil || r.status != http.StatusBadRequest {
		t.Fatalf("publish of an empty version: %v %v, want 400", r, err)
	}
}

var retryIn = regexp.MustCompile(`SSE listener: .*\(retry in (\d+s)\)`)

func stepResilience(t *testing.T, h *harness) {
	mark := h.proxy.mark()
	hold := h.proxy.holdResponse(http.MethodPost, "/api/sync/push")
	p := h.createPatient(t, h.clinicAdmin, "Systest", "Crash")
	if !hold.wait(converge) {
		t.Fatal("the clinic's push never reached the cloud")
	}

	// The cloud applied the push and answered; it dies before the answer
	// reaches the clinic, in the middle of the clinic's cycle.
	logBefore := len(h.clinic.logText())
	h.cloud.kill(t)
	hold.drop()
	eventually(t, converge, "the clinic's push failed", func() (bool, string) {
		return len(logLines(h.clinic.logText()[logBefore:], "[sync] push:")) > 0, "no push error yet"
	})

	// While the cloud is down the event stream retries with a growing delay.
	eventually(t, converge, "the stream backs off", func() (bool, string) {
		var seen []string
		for _, m := range retryIn.FindAllStringSubmatch(h.clinic.logText()[logBefore:], -1) {
			seen = append(seen, m[1])
		}
		return contains(seen, "2s"), strings.Join(seen, " ")
	})
	restarted := time.Now()
	h.cloud.start(t)

	// The clinic reconnects and pushes the row again; the cloud keeps one copy.
	eventually(t, converge, "push after the restart", func() (bool, string) {
		return len(requests(after(h.proxy.since(mark), restarted), http.MethodPost, "/api/sync/push")) > 0, "none yet"
	})
	for _, s := range []*session{h.clinicAdmin, h.cloudAdmin} {
		eventually(t, converge, "one crash patient on "+s.n.name, func() (bool, string) {
			n, err := s.totalOrError("/api/patients", "Systest Crash")
			return err == nil && n == 1, fmt.Sprint(n, err)
		})
	}
	h.cloudAdmin.waitStatus(t, "/api/balances/patient/"+p.ID, http.StatusOK)

	// The stream is back: the gate opens and cloud writes reach the clinic again.
	h.waitGateOpen(t, streamReconnect)
	h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/allergies", map[string]any{"name": "Systest after crash"})
	eventually(t, converge, "cloud write on the clinic after the restart", func() (bool, string) {
		n, err := h.clinicAdmin.totalOrError("/api/allergies", "Systest after crash")
		return err == nil && n == 1, fmt.Sprint(n, err)
	})
}

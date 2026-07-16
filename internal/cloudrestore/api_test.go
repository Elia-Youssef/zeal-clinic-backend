package cloudrestore

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/realtime"

	"github.com/labstack/echo/v4"
)

func TestHandleLocalAcceptsBodylessRequest(t *testing.T) {
	if buildmode.Cloud {
		t.Skip("local endpoint is not mounted in cloud builds")
	}
	e := echo.New()
	request := httptest.NewRequest(http.MethodPost, "/api/cloud-restore", nil)
	recorder := httptest.NewRecorder()
	if err := New(Config{}).HandleLocal(e.NewContext(request, recorder)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want configuration conflict instead of a body validation error", recorder.Code)
	}
}

func TestReportProgressBroadcastsSSEEvent(t *testing.T) {
	client := realtime.Register("cloud-restore-progress-test")
	defer client.Close()

	reportProgress("running", "applying", "Applying snapshot")
	select {
	case event := <-client.Events():
		if event.Type != progressEvent {
			t.Fatalf("event type = %q, want %q", event.Type, progressEvent)
		}
		progress, ok := event.Data.(Progress)
		if !ok {
			t.Fatalf("event data type = %T, want Progress", event.Data)
		}
		if progress.Status != "running" || progress.Stage != "applying" || progress.Message != "Applying snapshot" {
			t.Fatalf("progress = %#v", progress)
		}
	case <-time.After(time.Second):
		t.Fatal("progress event was not broadcast")
	}
}

func TestReportUploadProgressIncludesSnapshotSize(t *testing.T) {
	client := realtime.Register("cloud-restore-upload-progress-test")
	defer client.Close()

	const totalBytes = int64(5 * 1024 * 1024)
	reportUploadProgress(3, totalBytes)
	select {
	case event := <-client.Events():
		progress, ok := event.Data.(Progress)
		if !ok {
			t.Fatalf("event data type = %T, want Progress", event.Data)
		}
		if progress.Stage != "uploading" || progress.Step != 3 || progress.MaxSteps != 5 {
			t.Fatalf("progress = %#v", progress)
		}
		if progress.Message != "Uploading the local snapshot (5.0 MiB) and restoring cloud data" {
			t.Fatalf("message = %q", progress.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("upload progress event was not broadcast")
	}
}

func TestCloudProgressIncludesCloudStepCount(t *testing.T) {
	client := realtime.Register("cloud-restore-cloud-progress-test")
	defer client.Close()

	reportCloudProgress(3, "running", "backing_up", "Backing up the current cloud database")
	select {
	case event := <-client.Events():
		progress, ok := event.Data.(Progress)
		if !ok {
			t.Fatalf("event data type = %T, want Progress", event.Data)
		}
		if progress.Stage != "backing_up" || progress.Step != 3 || progress.MaxSteps != 6 {
			t.Fatalf("progress = %#v", progress)
		}
	case <-time.After(time.Second):
		t.Fatal("cloud progress event was not broadcast")
	}
}

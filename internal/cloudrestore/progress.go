package cloudrestore

import (
	"fmt"

	"clinic-api/internal/realtime"
)

const (
	progressEvent         = "cloud_restore_progress"
	localRestoreStepCount = 5
	cloudRestoreStepCount = 6
)

type Progress struct {
	Status   string `json:"status"`
	Stage    string `json:"stage"`
	Message  string `json:"message"`
	Step     int    `json:"step,omitempty"`
	MaxSteps int    `json:"maxSteps,omitempty"`
}

func reportProgress(status, stage, message string) {
	broadcastProgress(Progress{Status: status, Stage: stage, Message: message})
}

func reportLocalProgress(step int, status, stage, message string) {
	reportSteppedProgress(step, localRestoreStepCount, status, stage, message)
}

func reportCloudProgress(step int, status, stage, message string) {
	reportSteppedProgress(step, cloudRestoreStepCount, status, stage, message)
}

func reportSteppedProgress(step, maxSteps int, status, stage, message string) {
	broadcastProgress(Progress{
		Status:   status,
		Stage:    stage,
		Message:  message,
		Step:     step,
		MaxSteps: maxSteps,
	})
}

func reportUploadProgress(step int, totalBytes int64) {
	broadcastProgress(Progress{
		Status:   "running",
		Stage:    "uploading",
		Message:  fmt.Sprintf("Uploading the local snapshot (%s) and restoring cloud data", formatByteSize(totalBytes)),
		Step:     step,
		MaxSteps: localRestoreStepCount,
	})
}

func broadcastProgress(progress Progress) {
	realtime.Broadcast(realtime.Event{
		Type: progressEvent,
		Data: progress,
	})
}

func formatByteSize(bytes int64) string {
	const unit = int64(1024)
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	divisor := unit
	label := "KiB"
	for _, next := range []string{"MiB", "GiB", "TiB"} {
		if bytes < divisor*unit {
			break
		}
		divisor *= unit
		label = next
	}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(divisor), label)
}

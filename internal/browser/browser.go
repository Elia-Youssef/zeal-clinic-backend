package browser

import (
	"clinic-api/internal/config"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

func ProbeHealth() bool {
	port := config.Current().Port
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 250*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func WaitAndOpen() {
	for range 50 {
		if ProbeHealth() {
			Open()
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func Open() error {
	url := "http://localhost:" + config.Current().Port

	if runtime.GOOS == "windows" {
		cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		return cmd.Start()
	}

	return nil
}

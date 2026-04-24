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
	_ = conn.Close()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func WaitAndOpen() {
	url := "http://localhost:" + config.Current().Port
	for i := 0; i < 50; i++ {
		if ProbeHealth() {
			_ = Open(url)
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

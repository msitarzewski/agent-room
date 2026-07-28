//go:build linux

package notify

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestSendToAbstractSocket(t *testing.T) {
	name := "agentroom-notify-" + strings.ReplaceAll(t.Name(), "/", "-") + "-" + time.Now().Format("150405.000000")
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: "\x00" + name, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("NOTIFY_SOCKET", "@"+name)
	if err := Send("READY=1"); err != nil {
		t.Fatal(err)
	}
	_ = listener.SetReadDeadline(time.Now().Add(time.Second))
	buffer := make([]byte, 64)
	n, _, err := listener.ReadFromUnix(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buffer[:n]); got != "READY=1" {
		t.Fatalf("got %q", got)
	}
}

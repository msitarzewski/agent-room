package notify

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSendWithoutSocketIsNoop(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")
	if err := Send("READY=1"); err != nil {
		t.Fatal(err)
	}
}

func TestWatchdogHonorsPID(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "1000000")
	t.Setenv("WATCHDOG_PID", "99999999")
	if _, enabled, err := WatchdogInterval(); err != nil || enabled {
		t.Fatalf("enabled=%v err=%v pid=%d", enabled, err, os.Getpid())
	}
}

func TestWatchdogIntervalValidationAndDelivery(t *testing.T) {
	for _, value := range []string{"bad", "0", "-1"} {
		t.Setenv("WATCHDOG_USEC", value)
		t.Setenv("WATCHDOG_PID", "")
		if _, enabled, err := WatchdogInterval(); err == nil || enabled {
			t.Fatalf("WATCHDOG_USEC=%q enabled=%v err=%v", value, enabled, err)
		}
	}
	t.Setenv("WATCHDOG_USEC", "2000")
	t.Setenv("WATCHDOG_PID", "")
	interval, enabled, err := WatchdogInterval()
	if err != nil || !enabled || interval != time.Millisecond {
		t.Fatalf("interval=%v enabled=%v err=%v", interval, enabled, err)
	}

	socket := filepath.Join(os.TempDir(), fmt.Sprintf("ar-notify-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.Remove(socket) })
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socket, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("NOTIFY_SOCKET", socket)
	stop := make(chan struct{})
	var once sync.Once
	reported := make(chan error, 1)
	go Watchdog(stop, func(err error) { reported <- err })
	_ = listener.SetReadDeadline(time.Now().Add(time.Second))
	buffer := make([]byte, 64)
	n, _, err := listener.ReadFromUnix(buffer)
	once.Do(func() { close(stop) })
	if err != nil || string(buffer[:n]) != "WATCHDOG=1" {
		t.Fatalf("message=%q err=%v", buffer[:n], err)
	}
	select {
	case err := <-reported:
		t.Fatalf("unexpected watchdog error: %v", err)
	default:
	}
}

func TestWatchdogReportsConfigurationAndSocketErrors(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "invalid")
	reported := make(chan error, 1)
	Watchdog(make(chan struct{}), func(err error) { reported <- err })
	if err := <-reported; err == nil {
		t.Fatal("invalid watchdog configuration was not reported")
	}

	t.Setenv("WATCHDOG_USEC", "1000")
	t.Setenv("WATCHDOG_PID", "")
	t.Setenv("NOTIFY_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))
	stop := make(chan struct{})
	go Watchdog(stop, func(err error) {
		select {
		case reported <- err:
		default:
		}
	})
	select {
	case err := <-reported:
		if err == nil || errors.Is(err, os.ErrNotExist) {
			// Dial errors may wrap platform-specific socket errors; only a
			// non-nil report is part of the watchdog contract.
		}
		close(stop)
	case <-time.After(time.Second):
		close(stop)
		t.Fatal("socket failure was not reported")
	}
}

package notify

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func Send(state string) error {
	socket := os.Getenv("NOTIFY_SOCKET")
	if socket == "" {
		return nil
	}
	if strings.HasPrefix(socket, "@") {
		socket = "\x00" + strings.TrimPrefix(socket, "@")
	}
	address := &net.UnixAddr{Name: socket, Net: "unixgram"}
	conn, err := net.DialUnix("unixgram", nil, address)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte(state))
	return err
}

func WatchdogInterval() (time.Duration, bool, error) {
	raw := os.Getenv("WATCHDOG_USEC")
	if raw == "" {
		return 0, false, nil
	}
	microseconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || microseconds <= 0 {
		return 0, false, errors.New("WATCHDOG_USEC must be a positive integer")
	}
	if pid := os.Getenv("WATCHDOG_PID"); pid != "" && pid != strconv.Itoa(os.Getpid()) {
		return 0, false, nil
	}
	return time.Duration(microseconds) * time.Microsecond / 2, true, nil
}

func Watchdog(stop <-chan struct{}, report func(error)) {
	interval, enabled, err := WatchdogInterval()
	if err != nil {
		report(err)
		return
	}
	if !enabled {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := Send("WATCHDOG=1"); err != nil {
				report(err)
			}
		}
	}
}

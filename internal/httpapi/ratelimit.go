package httpapi

import (
	"container/list"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
	entry  *list.Element
}

type limiter struct {
	mu          sync.Mutex
	buckets     map[string]bucket
	lru         *list.List
	sockets     map[string]int
	now         func() time.Time
	maxKeys     int
	expiry      time.Duration
	nextCleanup time.Time
}

func newLimiter() *limiter {
	return newLimiterWithClock(time.Now, 4096)
}

func newLimiterWithClock(now func() time.Time, maxKeys int) *limiter {
	current := now()
	return &limiter{
		buckets: make(map[string]bucket), lru: list.New(), sockets: make(map[string]int),
		now: now, maxKeys: maxKeys, expiry: 10 * time.Minute, nextCleanup: current.Add(time.Minute),
	}
}

func (l *limiter) allow(key string, burst int, per time.Duration) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if !now.Before(l.nextCleanup) {
		l.removeExpired(now)
		l.nextCleanup = now.Add(time.Minute)
	}
	current, ok := l.buckets[key]
	if !ok {
		if l.maxKeys <= 0 {
			return false
		}
		if len(l.buckets) >= l.maxKeys {
			l.evictOldest()
		}
		current = bucket{tokens: float64(burst), last: now, entry: l.lru.PushBack(key)}
	} else {
		l.lru.MoveToBack(current.entry)
	}
	current.tokens += now.Sub(current.last).Seconds() * float64(burst) / per.Seconds()
	if current.tokens > float64(burst) {
		current.tokens = float64(burst)
	}
	current.last = now
	if current.tokens < 1 {
		l.buckets[key] = current
		return false
	}
	current.tokens--
	l.buckets[key] = current
	return true
}

func (l *limiter) removeExpired(now time.Time) {
	for {
		oldest := l.lru.Front()
		if oldest == nil {
			return
		}
		key := oldest.Value.(string)
		if now.Sub(l.buckets[key].last) <= l.expiry {
			return
		}
		delete(l.buckets, key)
		l.lru.Remove(oldest)
	}
}

func (l *limiter) evictOldest() {
	oldest := l.lru.Front()
	if oldest == nil {
		return
	}
	delete(l.buckets, oldest.Value.(string))
	l.lru.Remove(oldest)
}

func (l *limiter) acquireSocket(key string, maximum int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.sockets[key]; !exists && len(l.sockets) >= l.maxKeys {
		return false
	}
	if l.sockets[key] >= maximum {
		return false
	}
	l.sockets[key]++
	return true
}

func (l *limiter) releaseSocket(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sockets[key] <= 1 {
		delete(l.sockets, key)
	} else {
		l.sockets[key]--
	}
}

func remoteIP(request *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		peer := net.ParseIP(host)
		if request.TLS != nil && len(request.TLS.VerifiedChains) > 0 && trustedIP(peer, trusted) {
			if forwarded := firstForwardedIP(request.Header.Get("X-Forwarded-For")); forwarded != "" {
				return forwarded
			}
		}
		return peer.String()
	}
	return request.RemoteAddr
}

func trustedIP(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func firstForwardedIP(value string) string {
	if comma := strings.IndexByte(value, ','); comma >= 0 {
		value = value[:comma]
	}
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return ""
	}
	return ip.String()
}

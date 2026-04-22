package eks

import (
	"fmt"
	"sync"
	"time"
)

const (
	defaultEventDedupeTTL        = 30 * time.Minute
	defaultEventDedupeMaxEntries = 10000
)

type eventSignature struct {
	value      string
	recordedAt time.Time
}

type eventStateCache struct {
	mu         sync.Mutex
	last       map[string]eventSignature
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

var controllerEventState = &eventStateCache{
	last:       map[string]eventSignature{},
	ttl:        defaultEventDedupeTTL,
	maxEntries: defaultEventDedupeMaxEntries,
	now:        time.Now,
}

func (c *eventStateCache) shouldEmit(controllerName, namespace, name, eventType, reason, message string) bool {
	if c == nil {
		return true
	}

	key := fmt.Sprintf("%s/%s/%s", controllerName, namespace, name)
	signature := fmt.Sprintf("%s|%s|%s", eventType, reason, message)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.nowUTC()
	c.compactLocked(now)

	if prev, ok := c.last[key]; ok {
		if prev.value == signature && !c.isExpired(now, prev.recordedAt) {
			return false
		}
	}
	c.last[key] = eventSignature{
		value:      signature,
		recordedAt: now,
	}
	c.enforceSizeLocked()
	return true
}

func (c *eventStateCache) nowUTC() time.Time {
	if c == nil || c.now == nil {
		return time.Now().UTC()
	}
	return c.now().UTC()
}

func (c *eventStateCache) isExpired(now, recordedAt time.Time) bool {
	if c == nil || c.ttl <= 0 {
		return false
	}
	return now.Sub(recordedAt) > c.ttl
}

func (c *eventStateCache) compactLocked(now time.Time) {
	if c == nil || c.ttl <= 0 {
		return
	}
	for key, value := range c.last {
		if c.isExpired(now, value.recordedAt) {
			delete(c.last, key)
		}
	}
}

func (c *eventStateCache) enforceSizeLocked() {
	if c == nil || c.maxEntries <= 0 || len(c.last) <= c.maxEntries {
		return
	}
	excess := len(c.last) - c.maxEntries
	for range excess {
		oldestKey := ""
		oldestAt := time.Time{}
		for key, value := range c.last {
			if oldestKey == "" || value.recordedAt.Before(oldestAt) {
				oldestKey = key
				oldestAt = value.recordedAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(c.last, oldestKey)
	}
}

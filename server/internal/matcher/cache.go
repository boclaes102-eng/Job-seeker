package matcher

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// scoreCache is a process-local in-memory cache. Not persisted because scores
// depend on profile.md which can change at any time, and SQLite already stores
// the most recent score per job.
//
// The cache key incorporates the profile hash, so editing profile.md
// automatically invalidates everything.
type scoreCache struct {
	mu      sync.RWMutex
	entries map[string]cachedScore
}

type cachedScore struct {
	Score       int
	Reason      string
	MatchedTech []string
}

func newScoreCache() *scoreCache {
	return &scoreCache{entries: map[string]cachedScore{}}
}

func (c *scoreCache) key(jobURL, jobDesc, profileHash string) string {
	h := sha256.New()
	h.Write([]byte(jobURL))
	h.Write([]byte{0})
	h.Write([]byte(jobDesc))
	h.Write([]byte{0})
	h.Write([]byte(profileHash))
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:12])
}

func (c *scoreCache) get(key string) (cachedScore, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	return e, ok
}

func (c *scoreCache) put(key string, score int, reason string, matched []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cachedScore{Score: score, Reason: reason, MatchedTech: matched}
}

func (c *scoreCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]cachedScore{}
}

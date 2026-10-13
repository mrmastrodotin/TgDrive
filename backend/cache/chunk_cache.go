package cache

import (
	"container/list"
	"sync"
)

// ChunkCache is a thread-safe LRU cache for decrypted file chunks.
// It keeps recently-accessed plaintext chunks in memory so that
// repeated Range requests (audio seek, video scrub, PDF page turns)
// don't re-download from Telegram every time.
type ChunkCache struct {
	mu       sync.Mutex
	maxBytes int64
	curBytes int64
	items    map[string]*list.Element
	order    *list.List // front = most recently used
}

type cacheEntry struct {
	key  string
	data []byte
}

// New creates a ChunkCache with the given maximum byte capacity.
func New(maxBytes int64) *ChunkCache {
	return &ChunkCache{
		maxBytes: maxBytes,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Key builds a cache key from file ID and chunk index.
func Key(fileID string, chunkIndex int) string {
	// Avoid fmt.Sprintf overhead on the hot path
	buf := make([]byte, 0, len(fileID)+12)
	buf = append(buf, fileID...)
	buf = append(buf, ':')
	buf = appendInt(buf, chunkIndex)
	return string(buf)
}

func appendInt(b []byte, n int) []byte {
	if n == 0 {
		return append(b, '0')
	}
	if n < 0 {
		b = append(b, '-')
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return append(b, digits[i:]...)
}

// Get retrieves a cached chunk. Returns nil if not present.
func (c *ChunkCache) Get(key string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*cacheEntry).data
	}
	return nil
}

// Put stores a decrypted chunk in the cache, evicting LRU entries if necessary.
func (c *ChunkCache) Put(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := int64(len(data))

	// Already cached — update and promote
	if el, ok := c.items[key]; ok {
		old := el.Value.(*cacheEntry)
		c.curBytes -= int64(len(old.data))
		old.data = data
		c.curBytes += size
		c.order.MoveToFront(el)
		return
	}

	// Evict LRU entries until we have room
	for c.curBytes+size > c.maxBytes && c.order.Len() > 0 {
		c.evictOldest()
	}

	entry := &cacheEntry{key: key, data: data}
	el := c.order.PushFront(entry)
	c.items[key] = el
	c.curBytes += size
}

// evictOldest removes the least recently used entry. Caller must hold mu.
func (c *ChunkCache) evictOldest() {
	tail := c.order.Back()
	if tail == nil {
		return
	}
	entry := c.order.Remove(tail).(*cacheEntry)
	delete(c.items, entry.key)
	c.curBytes -= int64(len(entry.data))
}

// Clear removes all entries from the cache.
func (c *ChunkCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.order.Init()
	c.curBytes = 0
}

// Evict removes a specific key from the cache.
func (c *ChunkCache) Evict(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		entry := c.order.Remove(el).(*cacheEntry)
		delete(c.items, key)
		c.curBytes -= int64(len(entry.data))
	}
}

// Stats returns current cache utilization.
func (c *ChunkCache) Stats() (entries int, usedBytes, maxBytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len(), c.curBytes, c.maxBytes
}

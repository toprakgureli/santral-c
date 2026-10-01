package verimor

import (
	"sync"
	"time"
)

const (
	// maxRecording is the largest recording the panel plays; about two
	// hours of a phone call.
	maxRecording = 128 << 20
	// recordingKeep is how long a fetched recording stays in memory, so a
	// listener seeking back and forth does not fetch it again.
	recordingKeep = 10 * time.Minute
	// recordingCacheBytes bounds the memory the kept recordings may use.
	recordingCacheBytes = 256 << 20
)

// recordingCache keeps recently played recordings for a short while.
type recordingCache struct {
	mu    sync.Mutex
	items map[string]recordingItem
	size  int
}

type recordingItem struct {
	file    *RecordingFile
	expires time.Time
}

func newRecordingCache() *recordingCache {
	return &recordingCache{items: map[string]recordingItem{}}
}

func (c *recordingCache) get(uuid string) *RecordingFile {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[uuid]
	if !ok {
		return nil
	}
	if time.Now().After(it.expires) {
		c.drop(uuid)
		return nil
	}
	return it.file
}

func (c *recordingCache) put(uuid string, f *RecordingFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, it := range c.items {
		if now.After(it.expires) {
			c.drop(k)
		}
	}
	// Oldest first until the new one fits; a single oversized file is not kept.
	for c.size+len(f.Data) > recordingCacheBytes && len(c.items) > 0 {
		var oldest string
		var at time.Time
		for k, it := range c.items {
			if oldest == "" || it.expires.Before(at) {
				oldest, at = k, it.expires
			}
		}
		c.drop(oldest)
	}
	if len(f.Data) > recordingCacheBytes {
		return
	}
	if old, ok := c.items[uuid]; ok {
		c.size -= len(old.file.Data)
	}
	c.items[uuid] = recordingItem{file: f, expires: now.Add(recordingKeep)}
	c.size += len(f.Data)
}

// drop removes one entry; the caller holds the lock.
func (c *recordingCache) drop(uuid string) {
	if it, ok := c.items[uuid]; ok {
		c.size -= len(it.file.Data)
		delete(c.items, uuid)
	}
}

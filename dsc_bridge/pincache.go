package main

import "sync"

// PINCache holds token PINs in memory for the lifetime of the bridge process
// only. The chosen behaviour is "cache per session": the PIN is never written
// to disk and is wiped when the bridge exits. Under the autostart setup the
// bridge starts fresh on each login/reboot, so "session" == one login. This
// trades a little security (anyone at the unlocked machine can sign until the
// session ends) for not re-typing the PIN on every signature.
//
// Keyed by the certificate fingerprint the caller signs with, so multiple
// tokens each keep their own PIN independently.
type PINCache struct {
	mu   sync.Mutex
	pins map[string]string
}

// NewPINCache returns an empty in-memory PIN cache.
func NewPINCache() *PINCache {
	return &PINCache{pins: make(map[string]string)}
}

// Get returns the cached PIN for a fingerprint and whether one was present.
func (c *PINCache) Get(fingerprint string) (string, bool) {
	if fingerprint == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pin, ok := c.pins[fingerprint]
	return pin, ok
}

// Set stores a PIN for a fingerprint. No-ops on empty input.
func (c *PINCache) Set(fingerprint, pin string) {
	if fingerprint == "" || pin == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pins[fingerprint] = pin
}

// Forget drops the cached PIN for a fingerprint. Called after an incorrect or
// locked PIN so the next attempt re-prompts instead of reusing a bad value
// (which could otherwise burn retry attempts and lock the token).
func (c *PINCache) Forget(fingerprint string) {
	if fingerprint == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pins, fingerprint)
}

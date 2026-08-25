package ai

import (
	"fmt"
	"slices"
	"sync"
)

var keysMan *KeysManager

type KeysManager struct {
	mu         sync.RWMutex
	keys       []*APIKey
	currentIdx int
}

func (m *KeysManager) ChangeKey() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentIdx = (m.currentIdx + 1) % len(m.keys)
	return m.keys[m.currentIdx].Value()
}

func (m *KeysManager) GetKey() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.keys[m.currentIdx].Value()
}

func (m *KeysManager) Contains(val string) bool {
	return slices.ContainsFunc(m.keys, func(key *APIKey) bool { return key.val == val })
}

func (m *KeysManager) Add(val string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if val == "" {
		return fmt.Errorf("key cannot be empty")
	}

	for _, k := range m.keys {
		if k.val == val {
			return fmt.Errorf("key [%s] is already registered", FormatAPIKey(val))
		}
	}

	m.keys = append(m.keys, NewApiKey(val, 0))
	return nil
}

func (m *KeysManager) Remove(idx int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.keys)
	if n <= 1 {
		return fmt.Errorf("cannot remove last remaining key")
	}
	if idx < 0 || idx >= n {
		return fmt.Errorf("index out of bounds [0-%d]", n-1)
	}
	m.keys = append(m.keys[:idx], m.keys[idx+1:]...)
	if m.currentIdx >= len(m.keys) {
		m.currentIdx = 0
	}
	return nil
}

func (m *KeysManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]string, len(m.keys))
	for i, k := range m.keys {
		list[i] = k.Value()
	}
	return list
}

func (m *KeysManager) CurrentIndex() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentIdx
}

func initKeysManager(keys []string) error {
	keysLen := len(keys)
	if keysLen < 1 {
		return fmt.Errorf("keys array is empty")
	}
	man := &KeysManager{
		keys: make([]*APIKey, keysLen),
	}
	for i, key := range keys {
		man.keys[i] = NewApiKey(key, 0)
	}
	keysMan = man
	return nil
}

package typex

import "sync"

type Pool[T any] struct {
	pool *sync.Pool
}

func (p *Pool[T]) Put(v T) { p.pool.Put(v) }
func (p *Pool[T]) Get() T  { return p.pool.Get().(T) }

func NewPool[T any](newFunc func() T) Pool[T] {
	return Pool[T]{
		pool: &sync.Pool{New: func() any { return newFunc() }},
	}
}

// -----------------------------------
type Map[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

func (m *Map[K, V]) Has(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[key]
	return ok
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	return v, ok
}

func (m *Map[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

// locks map mutex; key present = returns v, false, else runs valueFunc() V sets v and returns v, true
func (m *Map[K, V]) SetIfAbsentLazy(key K, valueFunc func() V) (v V, ranFunc bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, exists := m.data[key]; exists {
		return existing, false
	}
	v = valueFunc()
	m.data[key] = v
	return v, true
}

func (m *Map[K, V]) SetIfAbsent(key K, value V) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.data[key]; exists {
		return false
	}
	m.data[key] = value
	return true
}

func (m *Map[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
}

func NewMap[K comparable, V any](size int) *Map[K, V] { return &Map[K, V]{data: make(map[K]V, size)} }

// ----------------------------
type Set[K comparable] struct {
	mu   sync.RWMutex
	data map[K]struct{}
}

func (s *Set[K]) Has(key K) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[key]
	return ok
}

func (s *Set[K]) Add(key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = struct{}{}
}

func (s *Set[K]) Delete(key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func NewSet[K comparable](size int) *Set[K] { return &Set[K]{data: make(map[K]struct{}, size)} }

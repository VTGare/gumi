package gumi

import "sync"

// Storage wraps a map[string]interface
// to provide thread safe access endpoints.
type Storage struct {
	mutex    sync.RWMutex
	innerMap map[string]interface{}
}

func newStorage() *Storage {
	return &Storage{
		innerMap: make(map[string]interface{}),
	}
}

// Get tries to get a value from the map.
func (s *Storage) Get(key string) (interface{}, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	v, ok := s.innerMap[key]
	return v, ok
}

// MustGet wraps Get but only returns the
// value, if found, or nil otherwise.
func (s *Storage) MustGet(key string) interface{} {
	v, ok := s.Get(key)
	if !ok {
		return nil
	}
	return v
}

// Set sets a value to the map by key.
func (s *Storage) Set(key string, val interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.innerMap[key] = val
}

// Delete removes a key-value pair from the map.
func (s *Storage) Delete(key string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.innerMap, key)
}

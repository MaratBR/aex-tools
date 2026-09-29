package secrets

import (
	"maps"
	"sync"
)

// Memory is a store that lives only in this process, starting with values. Plugins use it for
// the credentials aex passes them (internal/plugin), so they never open the real store.
func Memory(name string, values map[string]string) Store {
	return &memoryStore{name: name, m: maps.Clone(values)}
}

type memoryStore struct {
	name string
	mu   sync.Mutex
	m    map[string]string
}

func (s *memoryStore) Name() string { return s.name }

func (s *memoryStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key], nil
}

func (s *memoryStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = value
	return nil
}

func (s *memoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

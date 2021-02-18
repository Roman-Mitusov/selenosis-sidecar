package sidecar

import (
	"net/url"
	"sync"
)

type session struct {
	ID         string
	URL        *url.URL
	OnTimeout  chan struct{}
	CancelFunc func()
}

type Storage struct {
	sessions map[string]*session
	sync.RWMutex
}

//NewStorage ...
func NewStorage() *Storage {
	return &Storage{
		sessions: make(map[string]*session, 1),
	}
}

func (s *Storage) get(sessionID string) (*session, bool) {
	s.Lock()
	defer s.Unlock()
	sess, ok := s.sessions[sessionID]

	return sess, ok
}

func (s *Storage) put(sessionID string, sess *session) {
	s.Lock()
	defer s.Unlock()
	s.sessions[sessionID] = sess
}

//IsEmpty ...
func (s *Storage) IsEmpty() bool {
	s.Lock()
	defer s.Unlock()

	return len(s.sessions) == 0
}

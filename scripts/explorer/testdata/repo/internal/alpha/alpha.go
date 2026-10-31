// Package alpha is the first fixture package.
package alpha

import "example.com/fixture/internal/beta"

// Server serves the fixture.
type Server struct {
	// Addr is where it listens.
	Addr string
}

// Handler is implemented by fixture handlers.
type Handler interface {
	Serve() error
}

// Start starts the server.
func (s *Server) Start() error { return nil }

// Name names the fixture.
func Name() string { return beta.Name }

// Limit caps the fixture.
const Limit = 3

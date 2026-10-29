package alpha

import "net/http"

// routes registers the fixture's HTTP routes.
func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /widgets", s.requireOwner(s.listWidgets))
	mux.Handle("/health", http.HandlerFunc(s.health))
	for name, handler := range map[string]http.HandlerFunc{"b": s.health, "a": s.health} {
		mux.Handle("/files/"+name, handler)
	}
	for _, action := range []string{"start", "stop"} {
		mux.Handle("POST /jobs/"+action, s.requireOwner(s.listWidgets))
	}
	local := s.requireOwner(s.health)
	mux.Handle("/local", local)
	guarded := s.requireOwner(s.health)
	mux.Handle("/delegated", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guarded.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/open", func(w http.ResponseWriter, r *http.Request) {})
	twice := s.requireOwner(s.health)
	mux.HandleFunc("/twice", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			twice.ServeHTTP(w, r)
			return
		}
		twice.ServeHTTP(w, r)
	})
}

// requireOwner guards a handler.
func (s *Server) requireOwner(next http.HandlerFunc) http.HandlerFunc { return next }

func (s *Server) listWidgets(w http.ResponseWriter, r *http.Request) {}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {}

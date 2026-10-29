package alpha

import web "net/http"

// Mux builds a standalone mux.
func Mux() *web.ServeMux {
	mux := web.NewServeMux()
	mux.HandleFunc("/ping", func(w web.ResponseWriter, r *web.Request) {})
	return mux
}

// Default registers on the default mux.
func Default() { web.HandleFunc("/default", nil) }

type registry struct{}

// Handle records a name; it is not a ServeMux.
func (registry) Handle(name string, v any) {}

func notRoutes() {
	var reg registry
	reg.Handle("not-a-route", nil)
}

package main

import "net/http"

func main() {
	// hstsMiddleware sets HSTS headers for a given handler
	hstsMiddleware := func(h http.Handler) http.Handler {
		hf := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000") // 1 year
			h.ServeHTTP(w, r)
		}
		return http.HandlerFunc(hf)
	}
	_ = hstsMiddleware
}

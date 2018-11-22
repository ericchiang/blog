package main

import (
	"net/http"
	"time"
)

func main() {
	setCookie := func(w http.ResponseWriter, r *http.Request) {
		cookie := http.Cookie{
			Name:     "user_name",
			Value:    "ericchiang",
			Expires:  time.Now().Add(30 * 24 * time.Hour),
			HttpOnly: true, // Don't allow access from JavaScript
			Secure:   true, // Only send cookies over HTTPS
		}
		http.SetCookie(w, &cookie)
	}
	_ = setCookie
}

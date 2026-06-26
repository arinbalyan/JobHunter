package handler

import "net/http"

// ponytail: Vercel Go serverless function for /health.

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

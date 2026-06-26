package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

// ponytail: Vercel Go serverless function for /track endpoint.

func Handler(w http.ResponseWriter, r *http.Request) {
	emailID := r.URL.Query().Get("e")
	if emailID == "" {
		http.Error(w, "missing e param", 400)
		return
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		http.Error(w, "DATABASE_URL not set", 500)
		return
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer db.Close()

	_, _ = db.Exec(
		"UPDATE tracking SET opened = true, opened_at = COALESCE(opened_at, now()) WHERE email_id = $1",
		emailID,
	)

	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	// 1x1 transparent GIF
	w.Write([]byte{
		0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00,
		0x80, 0x00, 0x00, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00,
		0x21, 0xf9, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
		0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
	})
}

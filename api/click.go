package handler

import (
	"database/sql"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

// ponytail: Vercel Go serverless function for /click endpoint.

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
		"UPDATE tracking SET clicks = clicks + 1, last_clicked_at = now() WHERE email_id = $1",
		emailID,
	)

	http.Redirect(w, r, "https://linkedin.com/in/arinbalyan", 302)
}

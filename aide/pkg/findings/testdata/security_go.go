package testdata

import (
	"crypto/md5"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net/http"
	"os/exec"
)

// SQLInjection demonstrates string concatenation in SQL queries.
func SQLInjection(db *sql.DB, userID string) {
	db.Query("SELECT * FROM users WHERE id = " + userID)
}

// SQLSprintfInjection demonstrates fmt.Sprintf in SQL queries.
func SQLSprintfInjection(db *sql.DB, userID string) {
	query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", userID)
	db.Query(query)
}

// CommandInjection demonstrates exec.Command usage.
func CommandInjection(input string) {
	exec.Command("bash", "-c", input)
}

// ShellCommandInjection uses bash -c.
func ShellCommandInjection(cmd string) {
	exec.Command("bash", "-c", cmd)
}

// WeakHashMD5 uses MD5 hashing.
func WeakHashMD5(data []byte) {
	md5.Sum(data)
}

// InsecureTLS disables certificate verification.
func InsecureTLS() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
	}
}

// SafeQuery uses parameterized queries — should NOT trigger.
func SafeQuery(db *sql.DB, userID string) {
	db.Query("SELECT * FROM users WHERE id = ?", userID)
}

// SafeCommand uses exec.Command without user input — should trigger (we flag all exec.Command).
func SafeCommand() {
	exec.Command("ls", "-la")
}

// SSRFVulnerability uses user-controlled URL.
func SSRFVulnerability(r *http.Request) {
	url := r.URL.Query().Get("url")
	http.Get(url)
}

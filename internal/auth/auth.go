package auth

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/google/uuid"
)
const sessionCookie = "forum_session"

type Manager struct {
	db      *sql.DB
	maxAge  time.Duration
}

func NewManager(db *sql.DB, maxAge time.Duration) *Manager {
	return &Manager{db: db, maxAge: maxAge}
}

func (m *Manager) Create(w http.ResponseWriter, userID int64) error {
	id := uuid.New().String() 
	expires := time.Now().Add(m.maxAge)

	_, err := m.db.Exec(`INSERT INTO sessions(id,user_id,expires_at) VALUES(?,?,?)`, id, userID, expires)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
	return nil
}

func (m *Manager) Destroy(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(sessionCookie)
	if c != nil && c.Value != "" {
		m.db.Exec(`DELETE FROM sessions WHERE id = ?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Unix(0, 0),
	})
}

func (m *Manager) CurrentUserID(r *http.Request) (int64, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return 0, false
	}
	var uid int64
	var exp time.Time
	err = m.db.QueryRow(`SELECT user_id, expires_at FROM sessions WHERE id = ?`, c.Value).Scan(&uid, &exp)
	if err != nil || time.Now().After(exp) {
		return 0, false
	}
	return uid, true
}

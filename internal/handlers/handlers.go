package handlers

import (
	"database/sql"
	"errors"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"forum/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	db       *sql.DB
	sessions *auth.Manager
	tpls     *template.Template
}

func New(db *sql.DB, sessions *auth.Manager) *Handler {
	tpls := template.Must(template.ParseGlob(filepath.Join("web", "templates", "*.html")))
	return &Handler{db: db, sessions: sessions, tpls: tpls}
}

func (h *Handler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := h.sessions.CurrentUserID(r); !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (h *Handler) getTheme(r *http.Request) string {
	if c, err := r.Cookie("theme"); err == nil && (c.Value == "dark" || c.Value == "light") {
		return c.Value
	}
	return "light"
}

func (h *Handler) ToggleTheme(w http.ResponseWriter, r *http.Request) {
	cur := h.getTheme(r)
	newv := "dark"
	if cur == "dark" {
		newv = "light"
	}
	http.SetCookie(w, &http.Cookie{
		Name:    "theme",
		Value:   newv,
		Path:    "/",
		Expires: time.Now().Add(365 * 24 * time.Hour),
	})
	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

func (h *Handler) sidebarCats() []string {
	cats := []string{}
	cr, _ := h.db.Query(`SELECT name FROM categories ORDER BY name`)
	for cr.Next() {
		var name string
		cr.Scan(&name)
		cats = append(cats, name)
	}
	cr.Close()
	return cats
}

// -------- Pages

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		h.NotFound(w, r)
		return
	}

	q := `SELECT p.id, p.title, p.content, p.created_at, u.username,
		IFNULL((SELECT COUNT(*) FROM reactions WHERE target_type='post' AND target_id=p.id AND value=1),0) as likes,
		IFNULL((SELECT COUNT(*) FROM reactions WHERE target_type='post' AND target_id=p.id AND value=-1),0) as dislikes
		FROM posts p JOIN users u ON p.user_id=u.id`
	var args []any
	var joins []string
	var wheres []string

	if cat := r.URL.Query().Get("cat"); cat != "" {
		joins = append(joins, "JOIN post_categories pc ON pc.post_id=p.id JOIN categories c ON c.id=pc.category_id")
		wheres = append(wheres, "c.name = ?")
		args = append(args, cat)
	}

	uid, logged := h.sessions.CurrentUserID(r)
	if logged && r.URL.Query().Get("mine") == "1" {
		wheres = append(wheres, "p.user_id = ?")
		args = append(args, uid)
	}
	if logged && r.URL.Query().Get("liked") == "1" {
		joins = append(joins, "JOIN reactions rr ON rr.target_type='post' AND rr.target_id=p.id AND rr.user_id = ? AND rr.value=1")
		args = append(args, uid)
	}

	if len(joins) > 0 {
		q += " " + strings.Join(joins, " ")
	}
	if len(wheres) > 0 {
		q += " WHERE " + strings.Join(wheres, " AND ")
	}
	q += " ORDER BY p.created_at DESC LIMIT 200"

	rows, err := h.db.Query(q, args...)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type postVM struct {
		ID       int64
		Title    string
		Content  string
		Created  time.Time
		Author   string
		Likes    int
		Dislikes int
	}
	var posts []postVM
	for rows.Next() {
		var p postVM
		if err := rows.Scan(&p.ID, &p.Title, &p.Content, &p.Created, &p.Author, &p.Likes, &p.Dislikes); err != nil {
			http.Error(w, "Scan error", http.StatusInternalServerError)
			return
		}
		posts = append(posts, p)
	}

	h.tpls.ExecuteTemplate(w, "home", map[string]any{
    "Title":  "Forum",
    "Theme":  h.getTheme(r),
    "Logged": logged,
    "Posts":  posts,
    "Cats":   h.sidebarCats(),
    "Query":  r.URL.Query(),
})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.tpls.ExecuteTemplate(w, "register", map[string]any{
    "Title": "Register",
    "Theme": h.getTheme(r),
    "Cats":  h.sidebarCats(),
})
		return
	case http.MethodPost:
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	username := strings.TrimSpace(r.FormValue("username"))
	pass := r.FormValue("password")

	if email == "" || username == "" || pass == "" {
		http.Error(w, "All fields required", http.StatusBadRequest)
		return
	}

	hash, err := HashPassword(pass)
	if err != nil {
		http.Error(w, "hash error", http.StatusInternalServerError)
		return
	}

	_, err = h.db.Exec(`INSERT INTO users(email,username,password_hash,created_at) VALUES(?,?,?,?)`,
		email, username, hash, time.Now())
	if err != nil {
		http.Error(w, "Email or username already taken", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/login?registered=1", http.StatusSeeOther)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.tpls.ExecuteTemplate(w, "login", map[string]any{
    "Title":      "Login",
    "Theme":      h.getTheme(r),
    "Registered": r.URL.Query().Get("registered") == "1",
    "Cats":       h.sidebarCats(),
})
		return
	case http.MethodPost:
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	pass := r.FormValue("password")

	var id int64
	var hash string
	err := h.db.QueryRow(`SELECT id, password_hash FROM users WHERE email = ?`, email).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Wrong email or password", http.StatusUnauthorized)
		return
	} else if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	if !CheckPassword(pass, hash) {
		http.Error(w, "Wrong email or password", http.StatusUnauthorized)
		return
	}

	_, _ = h.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)

	if err := h.sessions.Create(w, id); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.sessions.Destroy(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) NewPost(w http.ResponseWriter, r *http.Request) {
	cats := []struct {
		ID   int64
		Name string
	}{}
	cr, _ := h.db.Query(`SELECT id,name FROM categories ORDER BY name`)
	for cr.Next() {
		var id int64
		var name string
		cr.Scan(&id, &name)
		cats = append(cats, struct {
			ID   int64
			Name string
		}{id, name})
	}
	cr.Close()

	h.tpls.ExecuteTemplate(w, "new_post", map[string]any{
    "Title":  "New Post",
    "Theme":  h.getTheme(r),
    "Cats":   cats,           // page list used by the form
    "Logged": true,
    // If your new_post template needs the sidebar list under a different key,
    // you can also pass it:
    "SidebarCats": h.sidebarCats(),
})

}

func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := h.sessions.CurrentUserID(r)

	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	if title == "" || content == "" {
		http.Error(w, "Title and content required", http.StatusBadRequest)
		return
	}

	res, err := h.db.Exec(`INSERT INTO posts(user_id,title,content,created_at) VALUES(?,?,?,?)`,
		uid, title, content, time.Now())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	pid, _ := res.LastInsertId()

	r.ParseForm()
	cats := r.Form["cats"]
	for _, c := range cats {
		var cid int64
		if err := h.db.QueryRow(`SELECT id FROM categories WHERE id=?`, c).Scan(&cid); err == nil {
			h.db.Exec(`INSERT OR IGNORE INTO post_categories(post_id,category_id) VALUES(?,?)`, pid, cid)
		}
	}

	http.Redirect(w, r, "/post/"+strconv.FormatInt(pid, 10), http.StatusSeeOther)
}

func (h *Handler) PostByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/post/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		h.NotFound(w, r)
		return
	}
	id, _ := strconv.ParseInt(parts[0], 10, 64)

	var title, content, author string
	var created time.Time
	err := h.db.QueryRow(`SELECT p.title,p.content,p.created_at,u.username 
		FROM posts p JOIN users u ON p.user_id=u.id WHERE p.id=?`, id).
		Scan(&title, &content, &created, &author)
	if errors.Is(err, sql.ErrNoRows) {
		h.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	cats := []string{}
	cRows, _ := h.db.Query(`SELECT c.name FROM categories c JOIN post_categories pc ON pc.category_id=c.id WHERE pc.post_id=?`, id)
	for cRows.Next() {
		var name string
		cRows.Scan(&name)
		cats = append(cats, name)
	}
	cRows.Close()

	var likes, dislikes int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM reactions WHERE target_type='post' AND target_id=? AND value=1`, id).Scan(&likes)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM reactions WHERE target_type='post' AND target_id=? AND value=-1`, id).Scan(&dislikes)

	type commentVM struct {
		ID       int64
		Content  string
		Author   string
		Created  time.Time
		Likes    int
		Dislikes int
	}
	var comments []commentVM
	rows, _ := h.db.Query(`SELECT c.id, c.content, u.username, c.created_at,
		IFNULL((SELECT COUNT(*) FROM reactions r WHERE r.target_type='comment' AND r.target_id=c.id AND r.value=1),0),
		IFNULL((SELECT COUNT(*) FROM reactions r WHERE r.target_type='comment' AND r.target_id=c.id AND r.value=-1),0)
		FROM comments c JOIN users u ON u.id=c.user_id WHERE c.post_id=? ORDER BY c.created_at`, id)
	for rows.Next() {
		var cm commentVM
		rows.Scan(&cm.ID, &cm.Content, &cm.Author, &cm.Created, &cm.Likes, &cm.Dislikes)
		comments = append(comments, cm)
	}
	rows.Close()

	_, logged := h.sessions.CurrentUserID(r)

h.tpls.ExecuteTemplate(w, "post", map[string]any{
    "Title":      title,
    "Theme":      h.getTheme(r),
    "PostID":     id,
    "Content":    content,
    "Author":     author,
    "Created":    created,
    "Categories": cats,
    "Likes":      likes,
    "Dislikes":   dislikes,
    "Comments":   comments,
    "Logged":     logged,
    "Cats":       h.sidebarCats(),
})

}

func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := h.sessions.CurrentUserID(r)
	postID, _ := strconv.ParseInt(r.FormValue("post_id"), 10, 64)
	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		http.Error(w, "Empty comments are not allowed", http.StatusBadRequest)
		return
	}
	_, err := h.db.Exec(`INSERT INTO comments(post_id,user_id,content,created_at) VALUES(?,?,?,?)`,
		postID, uid, content, time.Now())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/post/"+strconv.FormatInt(postID, 10), http.StatusSeeOther)
}

func (h *Handler) React(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, _ := h.sessions.CurrentUserID(r)
	targetType := r.FormValue("type")
	targetID, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	val := r.FormValue("value")
	iVal := -1
	if val == "1" {
		iVal = 1
	}

	_, err := h.db.Exec(`INSERT INTO reactions(user_id,target_type,target_id,value,created_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(user_id,target_type,target_id) DO UPDATE SET value=excluded.value, created_at=excluded.created_at`,
		uid, targetType, targetID, iVal, time.Now())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

func (h *Handler) MyPosts(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/?mine=1", http.StatusSeeOther)
}

func (h *Handler) MyLiked(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/?liked=1", http.StatusSeeOther)
}

func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
h.tpls.ExecuteTemplate(w, "notfound", map[string]any{
    "Title": "Not Found",
    "Theme": h.getTheme(r),
    "Cats":  h.sidebarCats(),
})
}

// --- password helpers (bcrypt) ---
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}
func CheckPassword(pw, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

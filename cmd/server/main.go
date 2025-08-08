package main

import (
    "log"
    "net/http"
    "os"
    "time"

    "forum/internal/auth"
    "forum/internal/db"
    "forum/internal/handlers"
)

func main() {
	// Create data dir for DB
	if err := os.MkdirAll("./data", 0755); err != nil {
		log.Fatal(err)
	}

	dbc, err := db.Open("./data/forum.db")
	if err != nil {
		log.Fatal(err)
	}
	defer dbc.Close()

	if err := db.Migrate(dbc); err != nil {
		log.Fatal(err)
	}

	sessions := auth.NewManager(dbc, 24*time.Hour)

	h := handlers.New(dbc, sessions)

	// static files
	fs := http.FileServer(http.Dir("./web/static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// routes
	http.HandleFunc("/", h.Index)
	http.HandleFunc("/register", h.Register)
	http.HandleFunc("/login", h.Login)
	http.HandleFunc("/logout", h.Logout)

	http.HandleFunc("/post/new", h.RequireAuth(h.NewPost))
	http.HandleFunc("/post/create", h.RequireAuth(h.CreatePost))
	http.HandleFunc("/post/", h.PostByID) // /post/{id}

	http.HandleFunc("/comment/create", h.RequireAuth(h.CreateComment))

	http.HandleFunc("/react", h.RequireAuth(h.React)) // like/dislike
	http.HandleFunc("/filter/mine", h.RequireAuth(h.MyPosts))
	http.HandleFunc("/filter/liked", h.RequireAuth(h.MyLiked))

	http.HandleFunc("/toggle-theme", h.ToggleTheme) // works for guests too

	// 404 fallback
	http.HandleFunc("/404", h.NotFound)

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	log.Printf("listening on %s", addr)
    // Wrap the default mux with a recovery middleware.  If any handler panics
    // the server will return a generic 500 response instead of crashing.  See
    // internal/handlers/middleware.go for details.
    err = http.ListenAndServe(addr, handlers.WithRecover(http.DefaultServeMux))
	if err != nil {
		log.Fatal(err)
	}
}

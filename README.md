# Forum

Minimal forum in Go + SQLite with server-rendered HTML/CSS.  
- Auth (single active session via cookies, UUID sessions, bcrypt passwords)  
- Posts, comments, categories, likes/dislikes  
- Filters (by category, my posts, liked posts)  
- Left vertical navbar with toggle, dark/light theme button (cookie), responsive, emojis & hover states  
- Dockerized

## Run (Docker)

```bash
./build.sh
# OR:
docker build -t forum .
docker run --rm -d -p 8080:8080 -v forum_data:/app/data --name forum forum

package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// ── Embed ──────────────────────────────────────────────────────────────
//
// The //go:embed directives bake templates, static assets, and posts
// directly into the compiled binary at build time.
//
// This means the Docker image is just a single static binary — no need
// to COPY loose files into the container or worry about working directories.
// Adding a new post is: write .md → commit → push → auto-deploy.

//go:embed templates/*.html
var templateFiles embed.FS

//go:embed static/*
var staticFiles embed.FS

//go:embed posts/*
var postFiles embed.FS

// ── Types ──────────────────────────────────────────────────────────────

type Post struct {
	Slug        string
	Title       string
	Date        string
	DatePretty  string
	Description string
	Content     template.HTML
}

type PageData struct {
	Title  string
	Active string
	Posts  []Post
	Post   Post
}

// ── Globals ────────────────────────────────────────────────────────────

var (
	md       goldmark.Markdown
	layout   *template.Template
	partials map[string]*template.Template
	funcMap  template.FuncMap
)

// ── Main ───────────────────────────────────────────────────────────────

func main() {
	md = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	funcMap = template.FuncMap{
		"currentYear": func() int { return time.Now().Year() },
	}

	// Parse templates from the embedded FS
	layout = template.Must(
		template.New("layout.html").Funcs(funcMap).ParseFS(templateFiles, "templates/layout.html"),
	)

	partialNames := []string{"home", "writing", "post", "misc"}
	partials = make(map[string]*template.Template)
	for _, name := range partialNames {
		cloned, err := layout.Clone()
		if err != nil {
			log.Fatalf("clone layout for %s: %v", name, err)
		}
		partials[name] = template.Must(
			cloned.ParseFS(templateFiles, "templates/"+name+".html"),
		)
	}

	// Serve static files from the embedded FS.
	// Strip the "static" prefix so /static/style.css maps to static/style.css
	// inside the embedded FS.
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("static sub fs: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/writing", handleWriting)
	mux.HandleFunc("/writing/", handlePost)
	mux.HandleFunc("/misc", handleMisc)

	// Cloud Run injects PORT; fall back to 8080 for local development.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// ── Handlers ───────────────────────────────────────────────────────────

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	render(w, r, "home", PageData{Title: "Taha Shafiei", Active: "home"})
}

func handleWriting(w http.ResponseWriter, r *http.Request) {
	posts, err := loadAllPosts()
	if err != nil {
		http.Error(w, "could not load posts", 500)
		log.Printf("loadAllPosts: %v", err)
		return
	}
	render(w, r, "writing", PageData{Title: "Writing | Taha Shafiei", Active: "writing", Posts: posts})
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/writing/")
	if slug == "" {
		http.Redirect(w, r, "/writing", http.StatusFound)
		return
	}
	post, err := loadPost(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	render(w, r, "post", PageData{Title: post.Title + " | Taha Shafiei", Active: "writing", Post: post})
}

func handleMisc(w http.ResponseWriter, r *http.Request) {
	render(w, r, "misc", PageData{Title: "Misc | Taha Shafiei", Active: "misc"})
}

// ── Render ─────────────────────────────────────────────────────────────

func render(w http.ResponseWriter, r *http.Request, name string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl, ok := partials[name]
	if !ok {
		http.NotFound(w, r)
		return
	}

	var err error
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Title", data.Title)
		err = tmpl.ExecuteTemplate(w, "content", data)
	} else {
		err = tmpl.ExecuteTemplate(w, "layout.html", data)
	}

	if err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

// ── Post loading ───────────────────────────────────────────────────────

func loadAllPosts() ([]Post, error) {
	entries, err := fs.ReadDir(postFiles, "posts")
	if err != nil {
		return nil, fmt.Errorf("readdir: %w", err)
	}
	var posts []Post
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p, err := loadPost(strings.TrimSuffix(e.Name(), ".md"))
		if err != nil {
			log.Printf("skip %s: %v", e.Name(), err)
			continue
		}
		posts = append(posts, p)
	}
	sort.Slice(posts, func(i, j int) bool { return posts[i].Date > posts[j].Date })
	return posts, nil
}

func loadPost(slug string) (Post, error) {
	if strings.Contains(slug, "..") || strings.ContainsAny(slug, "/\\") {
		return Post{}, fmt.Errorf("invalid slug")
	}

	// Read from the embedded FS instead of the real filesystem
	raw, err := postFiles.ReadFile(filepath.Join("posts", slug+".md"))
	if err != nil {
		return Post{}, err
	}

	meta, body := parseFrontmatter(string(raw))

	var buf bytes.Buffer
	if err := md.Convert([]byte(body), &buf); err != nil {
		return Post{}, err
	}

	date := meta["date"]
	return Post{
		Slug:        slug,
		Title:       meta["title"],
		Date:        date,
		DatePretty:  formatDate(date),
		Description: meta["description"],
		Content:     template.HTML(buf.String()),
	}, nil
}

// ── Frontmatter ────────────────────────────────────────────────────────

func parseFrontmatter(src string) (map[string]string, string) {
	meta := make(map[string]string)
	if !strings.HasPrefix(src, "---") {
		return meta, src
	}
	rest := strings.TrimLeft(strings.TrimPrefix(src, "---"), "\r\n")
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return meta, src
	}
	fm, body := rest[:idx], strings.TrimLeft(rest[idx+4:], "\r\n")

	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimRight(line, "\r")
		i := strings.Index(line, ":")
		if i == -1 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		meta[k] = v
	}
	return meta, body
}

// ── Utilities ──────────────────────────────────────────────────────────

func formatDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("January 2, 2006")
}

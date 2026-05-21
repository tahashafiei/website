package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// ── Data types ────────────────────────────────────────────────────────

// PostMeta holds the parsed frontmatter of a single post.
type PostMeta struct {
	Slug        string
	Title       string
	Date        string // "YYYY-MM-DD"
	DatePretty  string // "January 1, 2024"
	Description string
}

// Post embeds PostMeta and adds the rendered HTML body.
type Post struct {
	PostMeta
	ContentHTML template.HTML
}

// ── Markdown / frontmatter ────────────────────────────────────────────

// md is the shared goldmark instance — configured once at startup.
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,      // GitHub Flavoured Markdown (tables, strikethrough…)
		extension.Footnote, // [^1] footnotes
	),
	goldmark.WithRendererOptions(
		html.WithUnsafe(), // allow raw HTML inside .md files
	),
)

// parseFrontmatter splits a Markdown source file into:
//   - a map of key→value frontmatter fields (from the opening "---" block)
//   - the remaining body text
//
// It intentionally has no external dependencies — the format is simple
// enough to parse with strings.Cut and a range loop.
func parseFrontmatter(src string) (map[string]string, string) {
	fields := map[string]string{}

	// Frontmatter must start on the very first line.
	if !strings.HasPrefix(src, "---") {
		return fields, src
	}

	// Find the closing "---".
	rest := src[3:]
	// skip the newline after the opening ---
	if idx := strings.Index(rest, "\n"); idx >= 0 {
		rest = rest[idx+1:]
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return fields, src
	}

	block := rest[:end]
	body := rest[end+4:] // skip "\n---"
	// trim any leading newlines from body
	body = strings.TrimLeft(body, "\r\n")

	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimRight(line, "\r")
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// strip surrounding quotes
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		fields[key] = val
	}

	return fields, body
}

// formatDate converts "2024-06-01" → "June 1, 2024".
func formatDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("January 2, 2006")
}

// renderMarkdown converts a Markdown string to an HTML template.HTML value.
func renderMarkdown(src string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// ── Post loading ──────────────────────────────────────────────────────

const postsDir = "posts"

// loadAllPosts reads every .md file from postsDir and returns them
// sorted newest-first by date. Malformed files are skipped with a log.
func loadAllPosts() []PostMeta {
	entries, err := os.ReadDir(postsDir)
	if err != nil {
		log.Printf("loadAllPosts: %v", err)
		return nil
	}

	var posts []PostMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		slug := strings.TrimSuffix(e.Name(), ".md")
		src, err := os.ReadFile(filepath.Join(postsDir, e.Name()))
		if err != nil {
			log.Printf("loadAllPosts: read %s: %v", e.Name(), err)
			continue
		}

		fields, _ := parseFrontmatter(string(src))
		title := fields["title"]
		if title == "" {
			title = slug
		}
		date := fields["date"]

		posts = append(posts, PostMeta{
			Slug:        slug,
			Title:       title,
			Date:        date,
			DatePretty:  formatDate(date),
			Description: fields["description"],
		})
	}

	// Sort newest first (ISO dates sort lexicographically).
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Date > posts[j].Date
	})
	return posts
}

// loadPost reads a single post by slug and renders its Markdown body.
func loadPost(slug string) (*Post, error) {
	// Sanitise: reject path traversal and anything with separators.
	if strings.Contains(slug, "..") || strings.ContainsAny(slug, `/\`) {
		return nil, fmt.Errorf("invalid slug")
	}

	path := filepath.Join(postsDir, slug+".md")
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	fields, body := parseFrontmatter(string(src))
	title := fields["title"]
	if title == "" {
		title = slug
	}
	date := fields["date"]

	contentHTML, err := renderMarkdown(body)
	if err != nil {
		return nil, err
	}

	return &Post{
		PostMeta: PostMeta{
			Slug:        slug,
			Title:       title,
			Date:        date,
			DatePretty:  formatDate(date),
			Description: fields["description"],
		},
		ContentHTML: contentHTML,
	}, nil
}

// ── Templates ─────────────────────────────────────────────────────────

// templateData is the top-level data passed to every template render.
type templateData struct {
	Title       string     // browser <title>
	ActivePage  string     // "home" | "writing" | "misc"
	IsHTMX      bool       // true when the request came from HTMX
	Posts       []PostMeta // used by the writing index
	Post        *Post      // used by individual post pages
	CurrentYear int        // for the footer copyright
}

// render serves either a full page or an HTMX content fragment.
//
// Go's html/template shares a single namespace per template.Template,
// so if we parsed all .html files together with ParseGlob every file's
// {{define "content"}} block would collide — last one wins. Instead we
// parse layout.html + the specific partial for each request, giving each
// execution its own isolated namespace.
func render(w http.ResponseWriter, partial string, data templateData) {
	data.CurrentYear = time.Now().Year()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var (
		tmpl *template.Template
		name string
		err  error
	)

	if data.IsHTMX {
		// HTMX navigation: return only the {{define "content"}} block.
		// The client swaps it into <main> — no full page reload.
		tmpl, err = template.ParseFiles("templates/" + partial)
		name = "content"
	} else {
		// Full page or direct URL: render the complete layout.
		// layout.html calls {{template "content" .}} which is satisfied
		// by the partial's {{define "content"}} block.
		tmpl, err = template.ParseFiles("templates/layout.html", "templates/"+partial)
		name = "layout.html"
	}

	if err != nil {
		log.Printf("render parse %s: %v", partial, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	if err = tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render execute %s: %v", partial, err)
	}
}

// ── Handlers ──────────────────────────────────────────────────────────

// isHTMX checks for the HX-Request header that HTMX adds to every
// request it makes. When present, we return only the content fragment
// rather than the full HTML page.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	render(w, "home.html", templateData{
		Title:      "Taha Shafiei",
		ActivePage: "home",
		IsHTMX:     isHTMX(r),
	})
}

func handleWriting(w http.ResponseWriter, r *http.Request) {
	render(w, "writing.html", templateData{
		Title:      "Writing",
		ActivePage: "writing",
		IsHTMX:     isHTMX(r),
		Posts:      loadAllPosts(),
	})
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	post, err := loadPost(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	render(w, "post.html", templateData{
		Title:      post.Title,
		ActivePage: "writing",
		IsHTMX:     isHTMX(r),
		Post:       post,
	})
}

func handleMisc(w http.ResponseWriter, r *http.Request) {
	render(w, "misc.html", templateData{
		Title:      "Misc",
		ActivePage: "misc",
		IsHTMX:     isHTMX(r),
	})
}

// ── Main ──────────────────────────────────────────────────────────────

func main() {
	mux := http.NewServeMux()

	// Static files (CSS, favicon, etc.)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Pages
	mux.HandleFunc("GET /{$}", handleHome) // exact "/"
	mux.HandleFunc("GET /writing", handleWriting)
	mux.HandleFunc("GET /writing/{slug}", handlePost)
	mux.HandleFunc("GET /misc", handleMisc)

	addr := ":8080"
	log.Printf("Listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

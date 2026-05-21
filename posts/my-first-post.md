---
title: "First Post"
date: "2026-05-21"
description: "Obligatory first post."
---

Every blog should have a obligatory first post. This blog is mainly to talk about projects I have been working on.

Will it be updated frequently? Depends on my personal life and if I have people I can rant to in person.

## What to expect

Mostly technical writing. Occasionally some musing and ranting.

Unfortunately, personal blogs are not back and I never got into them in their hay day.

## The stack

This site is built with Go's standard library and HTMX. Posts are Markdown files on disk.
Navigation swaps only the page content — no full reloads.

```go
// The server detects HTMX requests via a header
func isHTMX(r *http.Request) bool {
    return r.Header.Get("HX-Request") == "true"
}
```

When `isHTMX` returns true, the server renders only the content partial.
When false (direct visit or hard refresh), it renders the full page layout.

The color scheme is [Kanagawa Dragon/Lotus](https://github.com/rebelot/kanagawa.nvim) (Dark and Light, respectively). Its easy on the eyes and kinda gives a ereader vibe to the site.

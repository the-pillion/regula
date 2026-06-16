package dashboard

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets/quill/*
var assetFS embed.FS

type templates struct {
	pages map[string]*template.Template
}

func loadTemplates() (*templates, error) {
	funcs := template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"fmtTime": func(t pgtype.Timestamptz) string {
			if !t.Valid {
				return ""
			}
			return t.Time.UTC().Format(time.RFC3339)
		},
		"fmtDate": func(t pgtype.Timestamptz) string {
			if !t.Valid {
				return ""
			}
			return t.Time.UTC().Format("2006-01-02")
		},
		"truncate": func(n int, s string) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "…"
		},
	}

	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}

	pages := map[string]*template.Template{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		name := e.Name()
		if name == "layout.html" || strings.HasSuffix(name, "_partial.html") {
			continue
		}
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(
			templateFS,
			"templates/layout.html",
			"templates/"+name,
		)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		pages[name] = t
	}

	return &templates{pages: pages}, nil
}

func (t *templates) render(w io.Writer, name string, data any) error {
	tpl, ok := t.pages[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	return tpl.ExecuteTemplate(w, "layout.html", data)
}

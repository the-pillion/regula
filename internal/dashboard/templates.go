package dashboard

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

// templates wraps parsed html/template sets keyed by page name.
// Each page is parsed together with layout.html so the page can
// override the {{define "page"}} block. Templates are loaded once
// at startup; they live in embed.FS so the binary is fully self-contained.
type templates struct {
	pages map[string]*template.Template
}

func loadTemplates() (*templates, error) {
	funcs := template.FuncMap{
		"upper": strings.ToUpper,
	}

	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}

	pages := map[string]*template.Template{}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "layout.html" || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(
			templateFS,
			"templates/layout.html",
			"templates/"+e.Name(),
		)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", e.Name(), err)
		}
		pages[e.Name()] = t
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

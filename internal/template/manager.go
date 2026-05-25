package template

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// TemplateData holds data available to all email templates.
type TemplateData struct {
	RecipientName  string
	CompanyName    string
	JobTitle       string
	JobURL         string
	Location       string
	IsRemote       bool
	TrackingID     string
	TrackingServer string
	UserName       string
	UserCurrentRole string
	UserYearsExp   int
	CustomFields   map[string]string
}

// Manager handles template rendering with embedded + override support.
type Manager struct {
	templates *template.Template
}

//go:embed embedded/*.gohtml
var embeddedTemplates embed.FS

// New creates a template manager.
// It loads embedded templates first, then checks the disk override dir.
func New(diskOverrideDir string) (*Manager, error) {
	funcMap := template.FuncMap{
		"add":      func(a, b int) int { return a + b },
		"subtract": func(a, b int) int { return a - b },
		"trackingPixel": func(server, id string) string {
			return fmt.Sprintf(`<img src="%s/track?id=%s" width="1" height="1" alt="" style="display:none;" />`, server, id)
		},
		"clickURL": func(server, id, url string) string {
			return fmt.Sprintf("%s/click?id=%s&url=%s", server, id, url)
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
	}

	// Start with embedded templates
	tmpl := template.New("").Funcs(funcMap)

	embedded, err := template.Must(tmpl.Clone()).Funcs(funcMap).ParseFS(embeddedTemplates, "embedded/*.gohtml")
	if err != nil {
		return nil, fmt.Errorf("parse embedded templates: %w", err)
	}
	tmpl = embedded

	// Override with disk templates if directory exists
	if diskOverrideDir != "" {
		if info, err := os.Stat(diskOverrideDir); err == nil && info.IsDir() {
			diskTmpl, err := template.Must(tmpl.Clone()).Funcs(funcMap).ParseGlob(filepath.Join(diskOverrideDir, "*.gohtml"))
			if err != nil {
				return nil, fmt.Errorf("parse disk templates: %w", err)
			}
			tmpl = diskTmpl
		}
	}

	return &Manager{templates: tmpl}, nil
}

// Render renders a named template with the given data.
func (m *Manager) Render(name string, data *TemplateData) (string, error) {
	var buf strings.Builder
	if err := m.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("execute template %q: %w", name, err)
	}
	return buf.String(), nil
}

// Available returns list of template names.
func (m *Manager) Available() []string {
	tmpls := m.templates.Templates()
	names := make([]string, 0, len(tmpls))
	for _, t := range tmpls {
		if name := t.Name(); name != "" {
			names = append(names, name)
		}
	}
	return names
}

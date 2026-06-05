// Package fallback provides Go text/template-based email generation
// when LLM providers are unavailable. Templates are tuned per experience match
// (underqualified, overqualified, qualified) to produce reasonable outreach.
package fallback

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
	"time"
)

//go:embed templates/*.txt
var templateFS embed.FS

// TemplateData holds all fields available to fallback templates.
type TemplateData struct {
	JobTitle       string
	Company        string
	JobDescription string
	Seniority      string
	Location       string
	JobType        string
	Salary         string
	Skills         string
	Industry       string
	ContactName    string
	ContactPhone   string
	ContactPortfolio string
	ContactGithub  string
	ContactLinkedin string
	ExperienceMatch string // "underqualified", "overqualified", "qualified"
}

// SubjectLine returns a reasonable subject line regardless of template.
func (d *TemplateData) SubjectLine() string {
	return fmt.Sprintf("Interested in %s role at %s", d.JobTitle, d.Company)
}

// Templates are loaded lazily on first use.
var (
	parsedTemplates map[string]*template.Template
	initErr         error
)

func initTemplates() {
	if parsedTemplates != nil || initErr != nil {
		return
	}

	parsedTemplates = make(map[string]*template.Template)
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		initErr = fmt.Errorf("read template dir: %w", err)
		return
	}

	funcMap := template.FuncMap{
		"now":   time.Now,
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		data, err := templateFS.ReadFile("templates/" + name)
		if err != nil {
			initErr = fmt.Errorf("read %s: %w", name, err)
			return
		}
		tmpl, err := template.New(name).Funcs(funcMap).Parse(string(data))
		if err != nil {
			initErr = fmt.Errorf("parse %s: %w", name, err)
			return
		}
		parsedTemplates[name] = tmpl
	}
}

// Generate produces an email body using the template matching the experience match.
// Returns (subject, body). If no matching template is found, uses a hardcoded fallback.
func Generate(data *TemplateData) (string, string) {
	subject := data.SubjectLine()

	// Ensure we have templates loaded
	initTemplates()
	if initErr != nil {
		return subject, hardcodedFallback(data)
	}

	// Map experience match to template file
	templateName := matchToTemplate(data.ExperienceMatch)
	tmpl, ok := parsedTemplates[templateName]
	if !ok {
		return subject, hardcodedFallback(data)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return subject, hardcodedFallback(data)
	}

	body := buf.String()
	if body == "" {
		return subject, hardcodedFallback(data)
	}

	return subject, body + contactBlock(data)
}

// contactBlock builds the standard contact footer.
func contactBlock(d *TemplateData) string {
	block := fmt.Sprintf("\n\n%s", d.ContactName)
	if d.ContactPhone != "" {
		block += "\nPhone: " + d.ContactPhone
	}
	if d.ContactPortfolio != "" {
		block += "\nPortfolio: " + d.ContactPortfolio
	}
	if d.ContactGithub != "" {
		block += "\nGitHub: " + d.ContactGithub
	}
	if d.ContactLinkedin != "" {
		block += "\nLinkedIn: " + d.ContactLinkedin
	}
	return block
}

// matchToTemplate maps experience match to template filename.
func matchToTemplate(match string) string {
	switch match {
	case "underqualified":
		return "underqualified.txt"
	case "overqualified":
		return "overqualified.txt"
	default:
		return "qualified.txt"
	}
}

// hardcodedFallback is used when templates fail to load or execute.
func hardcodedFallback(d *TemplateData) string {
	return fmt.Sprintf(
		"Hi %s team,\n\n"+
			"I came across your %s opening and wanted to reach out. "+
			"My background aligns well with what you're looking for. "+
			"I'd love to connect and discuss how I can contribute.\n\n"+
			"Best,%s",
		d.Company, d.JobTitle, contactBlock(d),
	)
}

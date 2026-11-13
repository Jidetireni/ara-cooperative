package email

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
)

// Templates are compiled into the binary so every deployment carries them.
//
//go:embed templates/*.html templates/*.txt
var templateFiles embed.FS

// Templates holds every email in HTML (wrapped in the shared layout) and plain text.
type Templates struct {
	html map[EmailTemplateType]*htmltemplate.Template
	text map[EmailTemplateType]*texttemplate.Template
}

var templateTypes = []EmailTemplateType{EmailTemplateTypeMemberInvitation, EmailTemplateTypeLoanGuarantorRequest}

// NewTemplates parses the embedded templates; it fails only if a template is malformed.
func NewTemplates() (*Templates, error) {
	t := &Templates{
		html: make(map[EmailTemplateType]*htmltemplate.Template, len(templateTypes)),
		text: make(map[EmailTemplateType]*texttemplate.Template, len(templateTypes)),
	}
	for _, name := range templateTypes {
		page, err := htmltemplate.ParseFS(templateFiles, "templates/layout.html", "templates/"+string(name)+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s html: %w", name, err)
		}
		plain, err := texttemplate.ParseFS(templateFiles, "templates/"+string(name)+".txt")
		if err != nil {
			return nil, fmt.Errorf("parse %s text: %w", name, err)
		}
		t.html[name] = page
		t.text[name] = plain
	}
	return t, nil
}

// Render produces both parts of one email.
func (t *Templates) Render(name EmailTemplateType, data any) (Rendered, error) {
	page, ok := t.html[name]
	if !ok {
		return Rendered{}, fmt.Errorf("unknown email template %q", name)
	}
	var htmlOut, textOut bytes.Buffer
	if err := page.ExecuteTemplate(&htmlOut, "layout", data); err != nil {
		return Rendered{}, fmt.Errorf("render %s html: %w", name, err)
	}
	if err := t.text[name].Execute(&textOut, data); err != nil {
		return Rendered{}, fmt.Errorf("render %s text: %w", name, err)
	}
	return Rendered{HTML: htmlOut.String(), Text: textOut.String()}, nil
}

var defaultTemplates = func() *Templates {
	t, err := NewTemplates()
	if err != nil {
		panic(err) // embedded files are fixed at build time; covered by tests
	}
	return t
}()

// Render uses the embedded templates.
func Render(name EmailTemplateType, data any) (Rendered, error) {
	return defaultTemplates.Render(name, data)
}

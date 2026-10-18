// Package templates holds the text of the emails and SMS, one directory per
// language: <locale>/<kind>.<channel>.tmpl, each defining "body" and, for
// email, "subject".
package templates

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
)

//go:embed */*.tmpl
var files embed.FS

// DefaultLocale is used when a message names no locale, or one without
// templates.
const DefaultLocale = "en"

// Renderer renders messages from the embedded templates. Parsed once at
// construction, so a broken template stops the service from starting instead
// of failing a send.
type Renderer struct {
	// by locale, then "<kind>.<channel>"
	templates map[string]map[string]*template.Template
}

func NewRenderer() (*Renderer, error) {
	locales, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("read templates: %w", err)
	}

	r := &Renderer{templates: make(map[string]map[string]*template.Template)}

	for _, locale := range locales {
		entries, err := fs.ReadDir(files, locale.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s templates: %w", locale.Name(), err)
		}

		r.templates[locale.Name()] = make(map[string]*template.Template)

		for _, entry := range entries {
			name := strings.TrimSuffix(entry.Name(), ".tmpl")

			content, err := fs.ReadFile(files, locale.Name()+"/"+entry.Name())
			if err != nil {
				return nil, fmt.Errorf("read template %s/%s: %w", locale.Name(), entry.Name(), err)
			}

			tmpl, err := template.New(name).Option("missingkey=error").Parse(string(content))
			if err != nil {
				return nil, fmt.Errorf("parse template %s/%s: %w", locale.Name(), entry.Name(), err)
			}

			r.templates[locale.Name()][name] = tmpl
		}
	}

	if r.templates[DefaultLocale] == nil {
		return nil, errors.New("no templates for the default locale " + DefaultLocale)
	}

	return r, nil
}

// Render uses msg.Locale, falling back to the default locale when it has no
// templates (or is empty).
func (r *Renderer) Render(msg domain.Message) (domain.RenderedMessage, error) {
	set, ok := r.templates[msg.Locale]
	if !ok {
		set = r.templates[DefaultLocale]
	}

	name := string(msg.Kind) + "." + string(msg.Channel)

	tmpl, ok := set[name]
	if !ok {
		return domain.RenderedMessage{}, fmt.Errorf("no template %q", name)
	}

	var rendered domain.RenderedMessage

	body, err := execute(tmpl, "body", msg.Data)
	if err != nil {
		return domain.RenderedMessage{}, err
	}
	rendered.Body = body

	if msg.Channel == domain.ChannelEmail {
		subject, err := execute(tmpl, "subject", msg.Data)
		if err != nil {
			return domain.RenderedMessage{}, err
		}
		rendered.Subject = subject
	}

	return rendered, nil
}

func execute(tmpl *template.Template, block string, data map[string]string) (string, error) {
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, block, data); err != nil {
		return "", fmt.Errorf("render %s of %s: %w", block, tmpl.Name(), err)
	}
	return out.String(), nil
}

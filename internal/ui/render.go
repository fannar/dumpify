package ui

import (
	"embed"
	"fmt"
	"html/template"
	"io"
)

//go:embed templates/*.html
var templateFS embed.FS

type Renderer struct {
	tpl *template.Template
}

func NewRenderer() (*Renderer, error) {
	tpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Renderer{tpl: tpl}, nil
}

func (r *Renderer) Render(w io.Writer, name string, data any) error {
	if err := r.tpl.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("render template %s: %w", name, err)
	}
	return nil
}

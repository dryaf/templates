// ==== File: integrations/chi/chi.go ====
package chi

import (
	"embed"
	"net/http"

	"github.com/dryaf/templates"
	"github.com/google/safehtml/template"
)

// Renderer provides a wrapper around templates.Templates for net/http compatible frameworks like chi.
type Renderer struct {
	*templates.Templates
}

// NewTemplatesRenderer creates a new Renderer with the given filesystem and function map.
// It is a convenience wrapper around templates.New.
func NewTemplatesRenderer(fs *embed.FS, fnMap template.FuncMap) *Renderer {
	return &Renderer{templates.New(fs, fnMap)}
}

// FromTemplates creates a new Renderer from an existing templates.Templates instance.
func FromTemplates(t *templates.Templates) *Renderer {
	return &Renderer{t}
}

// Render executes a template and writes the output to the http.ResponseWriter.
// It's designed to be used within a chi handler.
func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, name string, data interface{}) error {
	return r.ExecuteTemplate(w, req, name, data)
}

// Handler returns a http.HandlerFunc that renders the given template with the provided data.
// This can be used directly as a chi handler.
// If an error occurs, it will be logged by the underlying templates.Templates instance.
func (r *Renderer) Handler(templateName string, data interface{}) http.HandlerFunc {
	return r.HandlerRenderWithData(templateName, data)
}

// HandlerWithDataFromContext returns a http.HandlerFunc that renders the given template,
// using data from the request context. This can be used directly as a chi handler.
// If an error occurs, it will be logged by the underlying templates.Templates instance.
func (r *Renderer) HandlerWithDataFromContext(templateName string, contextKey interface{}) http.HandlerFunc {
	return r.HandlerRenderWithDataFromContext(templateName, contextKey)
}

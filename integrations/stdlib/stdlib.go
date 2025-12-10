// ==== File: integrations/stdlib/stdlib.go ====
// Package stdlib provides a convenience wrapper for using the templates engine
// with Go's standard net/http library.
package stdlib

import (
	"io/fs"
	"net/http"

	"github.com/dryaf/templates"
	"github.com/google/safehtml/template"
)

// Renderer provides a wrapper around templates.Templates for net/http.
type Renderer struct {
	*templates.Templates
}

// New creates a new Renderer with the given filesystem and function map.
// It is a convenience wrapper around templates.New, using functional options.
// Pass nil for fsys to use the local operating system filesystem. For production,
// an embed.FS is recommended.
func New(fsys fs.FS, fnMap template.FuncMap) *Renderer {
	return &Renderer{templates.New(templates.WithFileSystem(fsys), templates.WithFuncMap(fnMap))}
}

// FromTemplates creates a new Renderer from an existing templates.Templates instance.
// This is useful if you have already configured a templates.Templates instance.
func FromTemplates(t *templates.Templates) *Renderer {
	return &Renderer{t}
}

// Render executes a template, sets the HTTP status code, and writes the output
// to the http.ResponseWriter.
//
// Parameters:
//   - w: The http.ResponseWriter to write the rendered output to.
//   - req: The *http.Request, used to access context for layout selection.
//   - status: The HTTP status code to write.
//   - name: The template name to render, using the "layout:page" or ":page" syntax.
//   - data: The data to pass to the template.
func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, status int, name string, data interface{}) error {
	w.WriteHeader(status)
	return r.ExecuteTemplate(w, req, name, data)
}

// Handler returns a http.HandlerFunc that renders the given template with the provided data.
// If an error occurs during rendering, it will be logged by the underlying templates.Templates instance.
func (r *Renderer) Handler(templateName string, data interface{}) http.HandlerFunc {
	return r.HandlerRenderWithData(templateName, data)
}

// HandlerWithDataFromContext returns a http.HandlerFunc that renders the given template,
// using data from the request context.
// The data is retrieved from the request's context using the provided contextKey.
// If an error occurs, it will be logged by the underlying templates.Templates instance.
func (r *Renderer) HandlerWithDataFromContext(templateName string, contextKey interface{}) http.HandlerFunc {
	return r.HandlerRenderWithDataFromContext(templateName, contextKey)
}

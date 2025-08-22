// ==== File: integrations/chirender/chirender.go ====
// Package chirender provides an integration with the go-chi/render package,
// allowing seamless rendering of HTML templates alongside JSON/XML APIs.
package chirender

import (
	"net/http"

	"github.com/dryaf/templates"
	"github.com/go-chi/render"
)

// Template is a go-chi/render.Renderer implementation for the templates package.
// It wraps the core templates engine, the template name, and the data to be rendered.
type Template struct {
	Templates *templates.Templates
	Name      string
	Data      interface{}
}

// Render satisfies the render.Renderer interface. It sets the Content-Type header
// to "text/html" and executes the wrapped template. It also respects any status
// code previously set on the request context via render.Status().
func (t *Template) Render(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status, ok := r.Context().Value(render.StatusCtxKey).(int); ok {
		w.WriteHeader(status)
	}
	return t.Templates.ExecuteTemplate(w, r, t.Name, t.Data)
}

// New returns a new Template instance that implements render.Renderer.
// This is the primary way to prepare a template for rendering via render.Respond.
//
// Parameters:
//   - tmpls: A configured *templates.Templates instance.
//   - name: The name of the template to render (e.g., "home", "special:home").
//   - data: The data to pass to the template.
func New(tmpls *templates.Templates, name string, data interface{}) render.Renderer {
	return &Template{
		Templates: tmpls,
		Name:      name,
		Data:      data,
	}
}

// HTML is a custom responder for go-chi/render that handles the rendering of
// HTML templates. If the payload `v` is a *chirender.Template, it executes
// the template. Otherwise, it transparently falls back to the default
// go-chi/render responder, which is suitable for JSON/XML APIs.
//
// To use it, set it as the global responder for go-chi/render, typically once
// during application startup:
//
//	render.Respond = chirender.HTML
func HTML(w http.ResponseWriter, r *http.Request, v interface{}) {
	t, ok := v.(*Template)
	if !ok {
		render.DefaultResponder(w, r, v)
		return
	}

	if err := t.Render(w, r); err != nil {
		// The underlying templates engine logs the error, so we don't double-log.
		// Attempt to send an error response. This might fail if the template
		// has already started writing to the response writer.
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

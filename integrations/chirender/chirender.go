// ==== File: integrations/chirender/chirender.go ====
// Package chirender provides an integration with the go-chi/render package.
package chirender

import (
	"net/http"

	"github.com/dryaf/templates"
	"github.com/go-chi/render"
)

// Template is a go-chi/render Renderer for the templates package.
type Template struct {
	Templates *templates.Templates
	Name      string
	Data      interface{}
}

// Render satisfies the render.Renderer interface. It writes the content-type
// header and executes the template. It also respects the status code set by
// render.Status().
func (t *Template) Render(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status, ok := r.Context().Value(render.StatusCtxKey).(int); ok {
		w.WriteHeader(status)
	}
	return t.Templates.ExecuteTemplate(w, r, t.Name, t.Data)
}

// New returns a new Template instance that implements render.Renderer.
func New(tmpls *templates.Templates, name string, data interface{}) render.Renderer {
	return &Template{
		Templates: tmpls,
		Name:      name,
		Data:      data,
	}
}

// HTML is a custom responder for go-chi/render that handles rendering
// of HTML templates. If the payload `v` is a *chirender.Template, it executes
// the template. Otherwise, it falls back to the default go-chi/render responder,
// which is suitable for JSON/XML APIs.
//
// To use, set it as the responder for go-chi/render, typically once
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

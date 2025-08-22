// ==== File: integrations/gin/gin.go ====
// Package gin provides a renderer for the Gin framework.
package gin

import (
	"net/http"

	"github.com/dryaf/templates"
	"github.com/gin-gonic/gin/render"
)

// Renderer implements the gin.HTMLRenderer interface.
type Renderer struct {
	*templates.Templates
}

// New creates a new Renderer instance for Gin.
func New(tmpls *templates.Templates) *Renderer {
	return &Renderer{Templates: tmpls}
}

// Instance returns a gin.HTMLRender instance for a given template name and data.
func (r *Renderer) Instance(name string, data interface{}) render.Render {
	return &instance{
		Templates: r.Templates,
		Name:      name,
		Data:      data,
	}
}

// instance is a specific render instance for a single request.
type instance struct {
	Templates *templates.Templates
	Name      string
	Data      interface{}
}

// Render writes the template execution results to the writer.
// If an error occurs, it panics to allow Gin's recovery middleware to handle it.
func (i *instance) Render(w http.ResponseWriter) error {
	i.WriteContentType(w)

	// The gin render interface doesn't provide access to the http.Request,
	// so we pass nil. This means layout selection from context is not supported
	// in the Gin integration.
	err := i.Templates.ExecuteTemplate(w, nil, i.Name, i.Data)
	if err != nil {
		// The template engine logs detailed errors. A panic is the idiomatic way
		// to signal a 500 Internal Server Error to Gin's recovery middleware.
		panic(err)
	}
	return nil
}

// WriteContentType writes the Content-Type header.
func (i *instance) WriteContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

// ==== File: integrations/echo/echo.go ====
// Package echo provides a renderer and helper middleware for the Echo framework.
package echo

import (
	"io"

	"github.com/dryaf/templates"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// echoRenderer implements the echo.Renderer interface for the templates package.
type echoRenderer struct {
	*templates.Templates
}

// Render executes the template and writes its output to the provided writer.
// It is called by c.Render() in an Echo handler.
func (e *echoRenderer) Render(w io.Writer, name string, data interface{}, ctx echo.Context) error {
	return e.ExecuteTemplate(w, ctx.Request(), name, data)
}

// NewTemplatesRenderer creates a new echo.Renderer with the given options.
// It is a convenience wrapper around templates.New.
func NewTemplatesRenderer(opts ...templates.Option) echo.Renderer {
	return &echoRenderer{templates.New(opts...)}
}

// Renderer creates a new echo.Renderer from an existing templates.Templates instance.
// This is useful if you have already configured a templates.Templates instance.
func Renderer(t *templates.Templates) echo.Renderer {
	return &echoRenderer{t}
}

// MethodOverrideFormField is a convenience function that returns Echo's
// MethodOverride middleware configured to look for the method in a form field.
func MethodOverrideFormField(fieldName string) echo.MiddlewareFunc {
	return middleware.MethodOverrideWithConfig(middleware.MethodOverrideConfig{Getter: middleware.MethodFromForm(fieldName)})
}

// CSRFTokenLookup is a convenience function that returns Echo's CSRF middleware
// configured with the specified token lookup method (e.g., "form:_csrf").
func CSRFTokenLookup(lookupMethod string) echo.MiddlewareFunc {
	return middleware.CSRFWithConfig(middleware.CSRFConfig{TokenLookup: lookupMethod})
}

// ==== File: integrations/echo/echo.go ====
package echo

import (
	"embed"

	"io"

	"github.com/dryaf/templates"
	"github.com/google/safehtml/template"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type echoRenderer struct {
	*templates.Templates
}

func (e *echoRenderer) Render(w io.Writer, name string, data interface{}, ctx echo.Context) error {
	return e.ExecuteTemplate(w, ctx.Request(), name, data)
}

func NewTemplatesRenderer(fs *embed.FS, fnMap template.FuncMap) echo.Renderer {
	return &echoRenderer{templates.New(fs, fnMap)}
}

func Renderer(t *templates.Templates) echo.Renderer {
	return &echoRenderer{t}
}

func MethodOverrideFormField(fieldName string) echo.MiddlewareFunc {
	return middleware.MethodOverrideWithConfig(middleware.MethodOverrideConfig{Getter: middleware.MethodFromForm(fieldName)})
}

func CSRFTokenLookup(lookupMethod string) echo.MiddlewareFunc {
	return middleware.CSRFWithConfig(middleware.CSRFConfig{TokenLookup: lookupMethod})
}

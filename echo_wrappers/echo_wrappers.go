package echo_wrappers

import (
	"html/template"
	"io"
	"net/http"

	"github.com/dryaf/templates"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type echoRenderer struct {
	*templates.Templates
}

func (e *echoRenderer) Render(w io.Writer, name string, data interface{}, ctx echo.Context) error {
	return e.ExecuteTemplate(w, ctx.Request(), name, data)
}

func NewTemplatesRenderer(fs http.FileSystem, templatesPath string, fnMap template.FuncMap) echo.Renderer {
	return &echoRenderer{templates.New(fs, templatesPath, fnMap)}
}

func Renderer(t *templates.Templates) echo.Renderer {
	return &echoRenderer{t}
}

func MethodOverrideFormField(fieldName string) echo.MiddlewareFunc {
	return middleware.MethodOverrideWithConfig(middleware.MethodOverrideConfig{Getter: middleware.MethodFromForm("_method")})
}

func CSRFTokenLookup(lookupMethod string) echo.MiddlewareFunc {
	return middleware.CSRFWithConfig(middleware.CSRFConfig{TokenLookup: "form:csrf"})
}

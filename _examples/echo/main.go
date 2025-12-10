// ==== File: _examples/echo/main.go ====
package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"

	"github.com/dryaf/templates"
	templates_echo "github.com/dryaf/templates/integrations/echo"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed files
var embeddedFiles embed.FS

const useEmbeddedFS = false

// --- Data Structures ---
type Person struct {
	Name string
	Age  int
}

type CMSBlock struct {
	TemplateName string
	Data         interface{}
}

type CMSPageData struct {
	Title  string
	Blocks []CMSBlock
}

// --- Main Application ---
func main() {
	// --- Template Engine Setup ---
	var tmpls *templates.Templates
	if useEmbeddedFS {
		log.Println("Using embedded filesystem for templates (production mode)")
		tmpls = templates.New(&embeddedFiles, nil)
	} else {
		log.Println("Using local filesystem for templates (development mode)")
		tmpls = templates.New()
		tmpls.AlwaysReloadAndParseTemplates = true
	}
	tmpls.MustParseTemplates()

	// --- Mock Data ---
	personData := &Person{Name: "Echo", Age: 10}
	cmsData := &CMSPageData{
		Title: "Dynamic CMS Page (Echo)",
		Blocks: []CMSBlock{
			{TemplateName: "_header", Data: "Dynamic Content via Echo"},
			{TemplateName: "_user_card", Data: map[string]interface{}{"Name": "Edward", "Age": 28, "Status": "Active"}},
			{TemplateName: "_trusted_content", Data: "<code>This is safe, pre-formatted code.</code>"},
		},
	}
	homeData := map[string]string{
		"UnsafeHTML": "<div onclick='alert(`hacked`)'>Click me</div>",
		"SafeHTML":   "<span>This is safe.</span>",
	}

	// --- Echo Instance Setup ---
	e := echo.New()
	e.Renderer = templates_echo.Renderer(tmpls)
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// --- Routes ---
	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "home", homeData)
	})
	e.GET("/person", func(c echo.Context) error {
		return c.Render(http.StatusOK, "person", personData)
	})
	e.GET("/person-special", func(c echo.Context) error {
		return c.Render(http.StatusOK, "special:person", personData)
	})
	e.GET("/person-nolayout", func(c echo.Context) error {
		return c.Render(http.StatusOK, ":person", personData)
	})
	e.GET("/cms", func(c echo.Context) error {
		return c.Render(http.StatusOK, "cms_page", cmsData)
	})

	// --- Group with Middleware for Context Layout ---
	adminGroup := e.Group("/admin")
	adminGroup.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			ctx := context.WithValue(req.Context(), templates.LayoutContextKey{}, "special")
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	})
	adminGroup.GET("/dashboard", func(c echo.Context) error {
		return c.Render(http.StatusOK, "person", &Person{Name: "Admin (from Echo context)", Age: 103})
	})

	// --- Start Server ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}
	log.Printf("Starting server on http://localhost:%s (echo)", port)
	e.Logger.Fatal(e.Start(":" + port))
}

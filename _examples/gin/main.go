// ==== File: _examples/gin/main.go ====
package main

import (
	"embed"
	"log"
	"net/http"
	"os"

	"github.com/dryaf/templates"
	templates_gin "github.com/dryaf/templates/integrations/gin"
	"github.com/gin-gonic/gin"
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
		gin.SetMode(gin.DebugMode)
		tmpls = templates.New(nil, nil)
		tmpls.AlwaysReloadAndParseTemplates = true
	}
	tmpls.MustParseTemplates()

	// --- Mock Data ---
	personData := &Person{Name: "Ginnie", Age: 25}
	cmsData := &CMSPageData{
		Title: "Dynamic CMS Page (Gin)",
		Blocks: []CMSBlock{
			{TemplateName: "_header", Data: "Dynamic Content via Gin"},
			{TemplateName: "_user_card", Data: map[string]interface{}{"Name": "Giselle", "Age": 33, "Status": "Active"}},
			{TemplateName: "_trusted_content", Data: "A paragraph that is <em>trusted</em> and can contain HTML."},
		},
	}
	homeData := map[string]string{
		"UnsafeHTML": "<a href='javascript:void(0)'>unsafe</a>",
		"SafeHTML":   "<a href='/safe'>safe</a>",
	}

	// --- Gin Instance Setup ---
	router := gin.Default()
	router.HTMLRender = templates_gin.New(tmpls)

	// --- Routes ---
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "home", homeData)
	})
	router.GET("/person", func(c *gin.Context) {
		c.HTML(http.StatusOK, "person", personData)
	})
	router.GET("/person-special", func(c *gin.Context) {
		c.HTML(http.StatusOK, "special:person", personData)
	})
	router.GET("/person-nolayout", func(c *gin.Context) {
		c.HTML(http.StatusOK, ":person", personData)
	})
	router.GET("/cms", func(c *gin.Context) {
		c.HTML(http.StatusOK, "cms_page", cmsData)
	})

	// NOTE: Gin's c.HTML() render interface does not provide access to the
	// http.Request context, so middleware-based layout switching is not supported.
	// You must explicitly specify the layout in the render call.
	router.GET("/admin/dashboard", func(c *gin.Context) {
		c.HTML(http.StatusOK, "special:person", &Person{Name: "Admin (Gin explicit layout)", Age: 104})
	})

	// --- Start Server ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	log.Printf("Starting server on http://localhost:%s (gin)", port)
	router.Run(":" + port)
}

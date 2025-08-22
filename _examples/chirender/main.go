// ==== File: _examples/chirender/main.go ====
package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"

	"github.com/dryaf/templates"
	"github.com/dryaf/templates/integrations/chirender"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
)

//go:embed files
var embeddedFiles embed.FS

const useEmbeddedFS = false

// --- Data Structures ---
type Person struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
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
	// IMPORTANT: Set the custom HTML responder once at application startup.
	render.Respond = chirender.HTML

	// --- Template Engine Setup ---
	var tmpls *templates.Templates
	if useEmbeddedFS {
		log.Println("Using embedded filesystem for templates (production mode)")
		tmpls = templates.New(&embeddedFiles, nil)
	} else {
		log.Println("Using local filesystem for templates (development mode)")
		tmpls = templates.New(nil, nil)
		tmpls.AlwaysReloadAndParseTemplates = true
	}
	tmpls.MustParseTemplates()

	// --- Mock Data ---
	personData := &Person{Name: "ChiRender", Age: 55}
	cmsData := &CMSPageData{
		Title: "Dynamic CMS Page (ChiRender)",
		Blocks: []CMSBlock{
			{TemplateName: "_header", Data: "Dynamic Content via ChiRender"},
			{TemplateName: "_user_card", Data: map[string]interface{}{"Name": "Charles", "Age": 40, "Status": "Inactive"}},
			{TemplateName: "_trusted_content", Data: "This content is from a CMS and is <strong>trusted</strong>."},
		},
	}
	homeData := map[string]string{
		"UnsafeHTML": "<iframe src='javascript:alert(1)'></iframe>",
		"SafeHTML":   "<b>This is safe.</b>",
	}

	// --- Chi Router Setup ---
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Set a global default Content-Type for HTML responses.
	r.Use(render.SetContentType(render.ContentTypeHTML))

	// --- Routes ---
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		render.Respond(w, r, chirender.New(tmpls, "home", homeData))
	})

	// For this specific API route, use the .With() method to apply the JSON
	// content type middleware just for this handler.
	r.With(render.SetContentType(render.ContentTypeJSON)).Get("/api/person", func(w http.ResponseWriter, r *http.Request) {
		// The middleware sets the content type. render.Respond reads it from context
		// and correctly marshals the response as JSON.
		render.Respond(w, r, personData)
	})

	r.Get("/person-special", func(w http.ResponseWriter, r *http.Request) {
		render.Respond(w, r, chirender.New(tmpls, "special:person", personData))
	})
	r.Get("/person-nolayout", func(w http.ResponseWriter, r *http.Request) {
		render.Respond(w, r, chirender.New(tmpls, ":person", personData))
	})
	r.Get("/cms", func(w http.ResponseWriter, r *http.Request) {
		render.Respond(w, r, chirender.New(tmpls, "cms_page", cmsData))
	})
	r.With(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), templates.LayoutContextKey{}, "special")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}).Get("/admin/dashboard", func(w http.ResponseWriter, r *http.Request) {
		render.Respond(w, r, chirender.New(tmpls, "person", &Person{Name: "Admin (from ChiRender context)", Age: 102}))
	})

	// --- Start Server ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	log.Printf("Starting server on http://localhost:%s (chirender)", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal("ListenAndServe failed:", err)
	}
}

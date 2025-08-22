package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"

	"github.com/dryaf/templates"
	templates_chi "github.com/dryaf/templates/integrations/chi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
		tmpls = templates.New(nil, nil)
		tmpls.AlwaysReloadAndParseTemplates = true
	}
	tmpls.MustParseTemplates()
	template := templates_chi.FromTemplates(tmpls)

	// --- Mock Data ---
	personData := &Person{Name: "Chianti", Age: 50}
	cmsData := &CMSPageData{
		Title: "Dynamic CMS Page (Chi)",
		Blocks: []CMSBlock{
			{TemplateName: "_header", Data: "Dynamic Content via Chi"},
			{TemplateName: "_user_card", Data: map[string]interface{}{"Name": "Carlos", "Age": 35, "Status": "Pending"}},
			{TemplateName: "_trusted_content", Data: "<ul><li>Safe list item</li></ul>"},
		},
	}
	homeData := map[string]string{
		"UnsafeHTML": "<h1>unsafe</h1>",
		"SafeHTML":   "<h2>safe</h2>",
	}

	// --- Chi Router Setup ---
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// --- Routes ---
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, "home", homeData)
	})
	r.Get("/person", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, "person", personData)
	})
	r.Get("/person-special", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, "special:person", personData)
	})
	r.Get("/person-nolayout", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, ":person", personData)
	})
	r.Get("/cms", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, "cms_page", cmsData)
	})

	// --- Group with Middleware for Context Layout ---
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), templates.LayoutContextKey{}, "special")
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
		r.Get("/admin/dashboard", func(w http.ResponseWriter, r *http.Request) {
			template.Render(w, r, http.StatusOK, "person", &Person{Name: "Admin (from Chi context)", Age: 101})
		})
	})

	// --- Start Server ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	log.Printf("Starting server on http://localhost:%s (chi)", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal("ListenAndServe failed:", err)
	}
}

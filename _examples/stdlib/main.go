// ==== File: _examples/stdlib/main.go ====
package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"

	"github.com/dryaf/templates"
	"github.com/dryaf/templates/integrations/stdlib"
)

//go:embed files
var embeddedFiles embed.FS

// Set this to true to simulate a production environment using embedded files.
const useEmbeddedFS = false

// --- Data Structures for Templates ---
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
		// This check helps users diagnose if they haven't set up the examples correctly.
		if _, err := os.Stat("files/templates"); os.IsNotExist(err) {
			log.Fatalf("FATAL: 'files/templates' directory not found. Please run `make setup-examples` from the project root.")
		}
		log.Println("Using local filesystem for templates (development mode)")
		tmpls = templates.New(nil, nil)
		tmpls.AlwaysReloadAndParseTemplates = true
	}
	tmpls.MustParseTemplates()
	template := stdlib.FromTemplates(tmpls)

	// --- Mock Data ---
	personData := &Person{Name: "Bartholomew", Age: 42}
	cmsData := &CMSPageData{
		Title: "Dynamic CMS Page",
		Blocks: []CMSBlock{
			{TemplateName: "_header", Data: "Welcome to Our Dynamic Content"},
			{TemplateName: "_user_card", Data: map[string]interface{}{"Name": "Alice", "Age": 30, "Status": "Active"}},
			{TemplateName: "_trusted_content", Data: "<b>This is bold text</b> that is known to be safe."},
		},
	}
	homeData := map[string]string{
		"UnsafeHTML": "<script>alert('xss')</script>",
		"SafeHTML":   "<em>This HTML is from a trusted source.</em>",
	}

	// --- Middleware for Context-based Layout ---
	adminLayoutMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), templates.LayoutContextKey{}, "special")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	// --- Route Handlers ---
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, "home", homeData)
	})
	mux.HandleFunc("/person", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, "person", personData)
	})
	mux.HandleFunc("/person-special", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, "special:person", personData)
	})
	mux.HandleFunc("/person-nolayout", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, ":person", personData)
	})
	mux.HandleFunc("/cms", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, "cms_page", cmsData)
	})

	// --- Group with Middleware ---
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		template.Render(w, r, http.StatusOK, "person", &Person{Name: "Admin User", Age: 99})
	})
	mux.Handle("/admin/", http.StripPrefix("/admin", adminLayoutMiddleware(adminMux)))

	// --- Start Server ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting server on http://localhost:%s (stdlib)", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal("ListenAndServe failed:", err)
	}
}

// ==== File: integrations/chirender/chirender_test.go ====
package chirender

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dryaf/templates"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type Person struct {
	Name string
	Age  int
}

func TestMain(m *testing.M) {
	symlinkTarget := "../../files"
	symlinkName := "files"
	_ = os.Remove(symlinkName)
	if err := os.Symlink(symlinkTarget, symlinkName); err != nil {
		fmt.Printf("FATAL: Failed to create symlink for testing: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.Remove(symlinkName)
	os.Exit(code)
}

func setup(t *testing.T) *templates.Templates {
	tmpls := templates.New(nil, nil)
	tmpls.MustParseTemplates()
	return tmpls
}

func TestNew(t *testing.T) {
	tmpls := setup(t)
	payload := &Person{Name: "Test", Age: 100}
	renderer := New(tmpls, "person", payload)

	if renderer == nil {
		t.Fatal("New returned nil")
	}
	templateRenderer, ok := renderer.(*Template)
	if !ok {
		t.Fatal("New did not return a *Template")
	}
	if templateRenderer.Templates != tmpls {
		t.Error("Templates not set correctly")
	}
	if templateRenderer.Name != "person" {
		t.Errorf("Expected name 'person', got %s", templateRenderer.Name)
	}
	if templateRenderer.Data != payload {
		t.Error("Data not set correctly")
	}
}

func TestTemplate_Render(t *testing.T) {
	tmpls := setup(t)
	personData := &Person{Name: "Alice", Age: 30}
	templateRenderer := New(tmpls, "person", personData)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Test with render.Status
	render.Status(req, http.StatusAccepted)

	err := templateRenderer.Render(w, req)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status %d, got %d", http.StatusAccepted, w.Code)
	}
	if contentType := w.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type 'text/html; charset=utf-8', got '%s'", contentType)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Name: Alice") {
		t.Error("Body does not contain expected content")
	}
}

func TestHTMLResponder(t *testing.T) {
	tmpls := setup(t)
	personData := &Person{Name: "Bob", Age: 42}

	t.Run("renders template payload", func(t *testing.T) {
		templateRenderer := New(tmpls, "person", personData)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		HTML(w, req, templateRenderer)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "Name: Bob") {
			t.Error("Body does not contain expected content")
		}
	})

	t.Run("falls back to default responder for other types", func(t *testing.T) {
		apiResp := render.M{"message": "Success"}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "application/json")

		HTML(w, req, apiResp)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, `{"message":"Success"}`) {
			t.Errorf("Expected JSON response, got: %s", body)
		}
	})

	t.Run("handles render error", func(t *testing.T) {
		templateRenderer := New(tmpls, "nonexistent", nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		HTML(w, req, templateRenderer)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status Internal Server Error, got %d", w.Code)
		}
	})
}

// MockRenderer for testing fallback with other render.Renderer implementations.
type MockRenderer struct{}

func (m *MockRenderer) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func TestHTMLResponder_OtherRenderers(t *testing.T) {
	t.Run("falls back for non-Template renderers", func(t *testing.T) {
		mock := &MockRenderer{}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "application/json")

		// since mock is not *Template, it should fall back to DefaultResponder
		// which will try to JSON encode it.
		HTML(w, req, mock)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, `{}`) { // Empty JSON for the empty struct
			t.Errorf("Expected empty JSON response, got: %s", body)
		}
	})
}

func TestWithChiRouter(t *testing.T) {
	// Set the global responder for the test
	originalResponder := render.Respond
	render.Respond = HTML
	defer func() { render.Respond = originalResponder }()

	tmpls := setup(t)
	personData := &Person{Name: "ChiUser", Age: 28}

	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		render.Status(r, http.StatusCreated) // Test Status works
		render.Respond(w, r, New(tmpls, "person", personData))
	})
	r.Get("/fallback", func(w http.ResponseWriter, r *http.Request) {
		render.Respond(w, r, render.M{"hello": "world"})
	})

	t.Run("renders html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status code %d, but got %d", http.StatusCreated, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "Name: ChiUser") {
			t.Error("Expected person's name 'ChiUser' to be rendered")
		}
	})

	t.Run("fallback to json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/fallback", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, but got %d", http.StatusOK, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, `{"hello":"world"}`) {
			t.Errorf("Expected json response but got: %s", body)
		}
	})
}

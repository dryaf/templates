// ==== File: integrations/chi/chi_test.go ====
package chi

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dryaf/templates"
	"github.com/go-chi/chi/v5"
	"github.com/google/safehtml/template"
)

type Person struct {
	Name string
	Age  int
}

// TestMain creates a symlink to the project's template files before running tests,
// and removes it afterward. This allows the test to use the real, live templates
// without duplication and without compile-time embedding issues.
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

// setup creates and initializes a Renderer for testing.
func setup(t *testing.T) *Renderer {
	tmpls := templates.New(nil, nil)
	tmpls.MustParseTemplates()
	return FromTemplates(tmpls)
}

func TestNewTemplatesRendererAndFromTemplates(t *testing.T) {
	t.Run("NewTemplatesRenderer", func(t *testing.T) {
		renderer := NewTemplatesRenderer(nil, template.FuncMap{"testFunc": func() string { return "hello" }})
		if renderer == nil {
			t.Fatal("NewTemplatesRenderer returned nil")
		}
		if renderer.Templates == nil {
			t.Fatal("NewTemplatesRenderer did not initialize the embedded Templates instance")
		}
	})

	t.Run("FromTemplates", func(t *testing.T) {
		tmpls := templates.New(nil, nil)
		renderer := FromTemplates(tmpls)
		if renderer == nil {
			t.Fatal("FromTemplates returned nil")
		}
		if renderer.Templates != tmpls {
			t.Fatal("FromTemplates did not wrap the provided Templates instance")
		}
	})
}

func TestRenderer_Render(t *testing.T) {
	renderer := setup(t)
	personData := &Person{Name: "Alice", Age: 30}

	t.Run("renders page with default layout", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		err := renderer.Render(w, req, http.StatusAccepted, "person", personData)
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		if w.Code != http.StatusAccepted {
			t.Errorf("Expected status code %d, but got %d", http.StatusAccepted, w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "Layout-Full:") {
			t.Errorf("Expected default layout content 'Layout-Full:' in response, but it was not found")
		}
		if !strings.Contains(body, "Person-Page:") {
			t.Errorf("Expected page content 'Person-Page:' in response, but it was not found")
		}
		if !strings.Contains(body, "Name: Alice") {
			t.Errorf("Expected person's name 'Alice' in response, but it was not found")
		}
	})

	t.Run("renders page with specified layout", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		err := renderer.Render(w, req, http.StatusOK, "special:person", personData)
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		body := w.Body.String()
		if !strings.Contains(body, "Special-Layout:") {
			t.Errorf("Expected special layout content 'Special-Layout:' in response, but it was not found")
		}
		if !strings.Contains(body, "Person-Page:") {
			t.Errorf("Expected page content 'Person-Page:' in response, but it was not found")
		}
	})

	t.Run("renders page without layout", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		err := renderer.Render(w, req, http.StatusOK, ":person", personData)
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		body := w.Body.String()
		if strings.Contains(body, "Layout-Full:") {
			t.Errorf("Expected no layout content, but 'Layout-Full:' was found")
		}
		if !strings.Contains(body, "Person-Page:") {
			t.Errorf("Expected page content 'Person-Page:' in response, but it was not found")
		}
	})

	t.Run("renders using layout from request context", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		ctx := context.WithValue(req.Context(), templates.LayoutContextKey{}, "special")
		req = req.WithContext(ctx)

		err := renderer.Render(w, req, http.StatusOK, "person", personData)
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		body := w.Body.String()
		if !strings.Contains(body, "Special-Layout:") {
			t.Errorf("Expected layout from context 'Special-Layout:', but it was not found")
		}
		if strings.Contains(body, "Layout-Full:") {
			t.Errorf("Found default layout 'Layout-Full:' when special layout from context was expected")
		}
	})

	t.Run("returns error for non-existent template", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		err := renderer.Render(w, req, http.StatusNotFound, "nonexistent_page", nil)
		if err == nil {
			t.Fatal("Expected an error for a non-existent template, but got nil")
		}
		if !strings.Contains(err.Error(), "template: name not found") {
			t.Errorf("Expected error to contain 'template: name not found', but got: %v", err)
		}
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status code %d even on error, but got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestRenderer_Handler(t *testing.T) {
	renderer := setup(t)
	personData := &Person{Name: "Bob", Age: 42}

	handler := renderer.Handler("person", personData)

	req := httptest.NewRequest(http.MethodGet, "/person", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Layout-Full:") {
		t.Error("Expected default layout content")
	}
	if !strings.Contains(body, "Name: Bob") {
		t.Error("Expected person's name 'Bob' to be rendered")
	}
	if !strings.Contains(body, "Age: 42") {
		t.Error("Expected person's age '42' to be rendered")
	}
}

func TestRenderer_Handler_Error(t *testing.T) {
	renderer := setup(t)
	var logBuf bytes.Buffer
	renderer.Templates.Logger = slog.New(slog.NewTextHandler(&logBuf, nil))

	handler := renderer.Handler("nonexistent", nil)
	req := httptest.NewRequest(http.MethodGet, "/person", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "failed to execute template") || !strings.Contains(logOutput, "template_name=nonexistent") {
		t.Errorf("Expected log message on handler error, but got: %s", logOutput)
	}
}

func TestRenderer_HandlerWithDataFromContext(t *testing.T) {
	renderer := setup(t)
	personData := &Person{Name: "Charlie", Age: 55}

	type contextKey string
	const personKey contextKey = "person"

	handler := renderer.HandlerWithDataFromContext("person", personKey)

	req := httptest.NewRequest(http.MethodGet, "/person-from-context", nil)
	w := httptest.NewRecorder()

	ctx := context.WithValue(req.Context(), personKey, personData)
	req = req.WithContext(ctx)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Layout-Full:") {
		t.Error("Expected default layout content")
	}
	if !strings.Contains(body, "Name: Charlie") {
		t.Error("Expected person's name 'Charlie' from context to be rendered")
	}
	if !strings.Contains(body, "Age: 55") {
		t.Error("Expected person's age '55' from context to be rendered")
	}
}

func TestRenderer_HandlerWithDataFromContext_Error(t *testing.T) {
	renderer := setup(t)
	var logBuf bytes.Buffer
	renderer.Templates.Logger = slog.New(slog.NewTextHandler(&logBuf, nil))

	type contextKey string
	const personKey contextKey = "person"

	handler := renderer.HandlerWithDataFromContext("nonexistent", personKey)
	req := httptest.NewRequest(http.MethodGet, "/person-from-context", nil)
	w := httptest.NewRecorder()
	ctx := context.WithValue(req.Context(), personKey, &Person{})
	req = req.WithContext(ctx)

	handler.ServeHTTP(w, req)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "failed to execute template") || !strings.Contains(logOutput, "template_name=nonexistent") {
		t.Errorf("Expected log message on handler error, but got: %s", logOutput)
	}
}

func TestRenderer_WithChiRouter(t *testing.T) {
	renderer := setup(t)
	personData := &Person{Name: "ChiUser", Age: 28}

	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		err := renderer.Render(w, r, http.StatusOK, "person", personData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Name: ChiUser") {
		t.Error("Expected person's name 'ChiUser' to be rendered")
	}
}

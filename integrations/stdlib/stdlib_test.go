// ==== File: integrations/stdlib/stdlib_test.go ====
package stdlib

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
	"github.com/google/safehtml/template"
)

// NOTE: We are not using go:embed in this test file anymore.
// Instead, we create a symlink to the real templates at runtime.

type Person struct {
	Name string
	Age  int
}

// TestMain creates a symlink to the project's template files before running tests,
// and removes it afterward. This allows the test to use the real, live templates
// without duplication and without compile-time embedding issues.
func TestMain(m *testing.M) {
	// The target is the actual 'files' directory at the project root.
	symlinkTarget := "../../files"
	// The name of the symlink we will create in the current directory.
	symlinkName := "files"

	// Clean up any old symlink from a previously failed test run.
	_ = os.Remove(symlinkName) // Use os.Remove, not RemoveAll

	// Create the symlink.
	if err := os.Symlink(symlinkTarget, symlinkName); err != nil {
		fmt.Printf("FATAL: Failed to create symlink for testing: %v\n", err)
		os.Exit(1)
	}

	// Run all tests.
	code := m.Run()

	// Clean up the symlink after tests are done.
	if err := os.Remove(symlinkName); err != nil { // Use os.Remove, not RemoveAll
		fmt.Printf("WARNING: Failed to clean up symlink: %v\n", err)
	}

	os.Exit(code)
}

// setup creates and initializes a Renderer for testing.
func setup(t *testing.T) *Renderer {
	// By passing nil, templates.New() will use the local filesystem.
	// Thanks to the symlink created in TestMain, it will find and read
	// the project's actual template files from './files/templates/...'.
	tmpls := templates.New()
	tmpls.MustParseTemplates()
	return FromTemplates(tmpls)
}

func TestNewTemplatesRendererAndFromTemplates(t *testing.T) {
	t.Run("NewTemplatesRenderer", func(t *testing.T) {
		// We pass a nil embed.FS because we are testing construction, not parsing.
		renderer := New(nil, template.FuncMap{"testFunc": func() string { return "hello" }})
		if renderer == nil {
			t.Fatal("NewTemplatesRenderer returned nil")
		}
		if renderer.Templates == nil {
			t.Fatal("NewTemplatesRenderer did not initialize the embedded Templates instance")
		}
	})

	t.Run("FromTemplates", func(t *testing.T) {
		// Create a templates instance manually, which will use the filesystem.
		tmpls := templates.New()
		// Wrap it with the renderer
		renderer := FromTemplates(tmpls)
		if renderer == nil {
			t.Fatal("FromTemplates returned nil")
		}
		// Check if it points to the original instance
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

		// Add the layout key to the request's context
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
		if !strings.Contains(err.Error(), templates.ErrTemplateNotFound.Error()) {
			t.Errorf("Expected error to contain '%v', but got: %v", templates.ErrTemplateNotFound, err)
		}
		// The header is written before the error is found. This is expected.
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status code %d even on error, but got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestRenderer_Handler(t *testing.T) {
	renderer := setup(t)
	personData := &Person{Name: "Bob", Age: 42}

	// Create a handler with static data
	handler := renderer.Handler("person", personData)

	req := httptest.NewRequest(http.MethodGet, "/person", nil)
	w := httptest.NewRecorder()

	// Execute the handler
	handler.ServeHTTP(w, req)

	// Assertions
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

	// Create a handler that pulls data from the context
	handler := renderer.HandlerWithDataFromContext("person", personKey)

	req := httptest.NewRequest(http.MethodGet, "/person-from-context", nil)
	w := httptest.NewRecorder()

	// Create a context with the person data and attach it to the request
	ctx := context.WithValue(req.Context(), personKey, personData)
	req = req.WithContext(ctx)

	// Execute the handler
	handler.ServeHTTP(w, req)

	// Assertions
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

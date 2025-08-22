// ==== File: integrations/gin/gin_test.go ====
package gin

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dryaf/templates"
	"github.com/gin-gonic/gin"
)

type Person struct {
	Name string
	Age  int
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
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

func setup(t *testing.T) (*gin.Engine, *templates.Templates) {
	tmpls := templates.New(nil, nil)
	tmpls.MustParseTemplates()

	// Use gin.Default() to include the recovery middleware, which is essential
	// for handling panics from the renderer.
	r := gin.Default()
	r.HTMLRender = New(tmpls)
	return r, tmpls
}

func TestNew(t *testing.T) {
	tmpls := templates.New(nil, nil)
	renderer := New(tmpls)
	if renderer == nil {
		t.Fatal("New returned nil")
	}
	if renderer.Templates != tmpls {
		t.Error("Templates not set correctly on the renderer")
	}
}

func TestRender(t *testing.T) {
	r, _ := setup(t)
	personData := &Person{Name: "GinUser", Age: 33}

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "person", personData)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Layout-Full:") {
		t.Error("Expected default layout content")
	}
	if !strings.Contains(body, "Name: GinUser") {
		t.Error("Expected person's name 'GinUser' to be rendered")
	}
	if !strings.Contains(body, "Age: 33") {
		t.Error("Expected person's age '33' to be rendered")
	}
	if contentType := w.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("Expected Content-Type to start with 'text/html', but got '%s'", contentType)
	}
}

func TestRenderWithLayout(t *testing.T) {
	r, _ := setup(t)
	r.GET("/special", func(c *gin.Context) {
		c.HTML(http.StatusOK, "special:person", &Person{Name: "SpecialGin", Age: 44})
	})

	req := httptest.NewRequest(http.MethodGet, "/special", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Special-Layout:") {
		t.Error("Expected special layout content")
	}
	if !strings.Contains(body, "Name: SpecialGin") {
		t.Error("Expected person's name 'SpecialGin' to be rendered")
	}
}

func TestRenderError(t *testing.T) {
	r, tmpls := setup(t)

	// Suppress logging for this test to avoid noisy output
	originalLogger := tmpls.Logger
	tmpls.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	defer func() { tmpls.Logger = originalLogger }()

	r.GET("/error", func(c *gin.Context) {
		c.HTML(http.StatusOK, "nonexistent", nil)
	})

	// Gin's recovery middleware will catch the panic and respond with 500.
	// We check for that status code.
	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d on render error, but got %d", http.StatusInternalServerError, w.Code)
	}
}

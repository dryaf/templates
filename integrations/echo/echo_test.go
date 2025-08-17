// ==== File: integrations/echo/echo_test.go ====
package echo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/dryaf/templates"
	"github.com/labstack/echo/v4"
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
	_ = os.RemoveAll(symlinkName)

	// Create the symlink.
	if err := os.Symlink(symlinkTarget, symlinkName); err != nil {
		fmt.Printf("FATAL: Failed to create symlink for testing: %v\n", err)
		os.Exit(1)
	}

	// Run all tests.
	code := m.Run()

	// Clean up the symlink after tests are done.
	if err := os.RemoveAll(symlinkName); err != nil {
		fmt.Printf("WARNING: Failed to clean up symlink: %v\n", err)
	}

	os.Exit(code)
}

// setup creates and initializes an echo.Renderer and echo.Echo instance for testing.
func setup(t *testing.T) (*echoRenderer, *echo.Echo) {
	// By passing nil, templates.New() will use the local filesystem.
	// Thanks to the symlink created in TestMain, it will find and read
	// the project's actual template files from './files/templates/...'.
	tmpls := templates.New(nil, nil)
	tmpls.MustParseTemplates()

	e := echo.New()
	renderer := Renderer(tmpls).(*echoRenderer)
	e.Renderer = renderer

	return renderer, e
}

func TestNewTemplatesRendererAndRenderer(t *testing.T) {
	// This test is now less critical as NewTemplatesRenderer is for embed.FS,
	// but we can ensure it still constructs an object.
	t.Run("NewTemplatesRenderer", func(t *testing.T) {
		// We pass a nil embed.FS because we are testing construction, not parsing.
		renderer := NewTemplatesRenderer(nil, nil)
		if renderer == nil {
			t.Fatal("NewTemplatesRenderer returned nil")
		}
	})

	t.Run("Renderer", func(t *testing.T) {
		tmpls := templates.New(nil, nil)
		renderer := Renderer(tmpls).(*echoRenderer)
		if renderer.Templates != tmpls {
			t.Fatal("Renderer did not wrap the provided Templates instance")
		}
	})
}

func TestEchoRenderer_Render(t *testing.T) {
	renderer, e := setup(t)
	personData := &Person{Name: "EchoUser", Age: 25}

	t.Run("renders page with default layout via echo context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := c.Render(http.StatusOK, "person", personData)
		if err != nil {
			t.Fatalf("c.Render failed: %v", err)
		}

		body := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}
		if !strings.Contains(body, "Layout-Full:") {
			t.Error("Expected default layout content")
		}
		if !strings.Contains(body, "Name: EchoUser") {
			t.Error("Expected person's name to be rendered")
		}
	})

	t.Run("renders using layout from request context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		ctx := context.WithValue(c.Request().Context(), templates.LayoutContextKey{}, "special")
		c.SetRequest(c.Request().WithContext(ctx))

		err := renderer.Render(rec, "person", personData, c)
		if err != nil {
			t.Fatalf("renderer.Render failed: %v", err)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "Special-Layout:") {
			t.Error("Expected special layout from context")
		}
	})

	t.Run("returns error for non-existent template", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := renderer.Render(rec, "nonexistent", nil, c)
		if err == nil {
			t.Fatal("Expected an error for a non-existent template, but got nil")
		}
	})
}

func TestMethodOverrideFormField(t *testing.T) {
	e := echo.New()
	e.Use(MethodOverrideFormField("_method"))

	handler := func(c echo.Context) error {
		if c.Request().Method != http.MethodDelete {
			t.Errorf("Expected method to be DELETE, but got %s", c.Request().Method)
		}
		return c.String(http.StatusOK, "deleted")
	}

	// Register a handler for the original method (POST) and the target (DELETE)
	// so the router can find the route before the middleware runs.
	e.POST("/test", handler)
	e.DELETE("/test", handler)

	form := url.Values{}
	form.Add("_method", "DELETE")
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", rec.Code)
	}
	if rec.Body.String() != "deleted" {
		t.Errorf("Expected body 'deleted', got '%s'", rec.Body.String())
	}
}

func TestCSRFTokenLookup(t *testing.T) {
	e := echo.New()
	e.Use(CSRFTokenLookup("form:csrf"))

	e.GET("/form", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.POST("/submit", func(c echo.Context) error {
		return c.String(http.StatusOK, "submitted")
	})

	// 1. GET request to establish CSRF cookie
	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /form failed with status %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("Expected CSRF cookie to be set, but found none")
	}
	csrfCookie := cookies[0]
	token := csrfCookie.Value

	// 2. Successful POST with the correct token
	form := url.Values{}
	form.Add("csrf", token)
	req = httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(csrfCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status OK on POST with valid token, got %d", rec.Code)
	}
	if rec.Body.String() != "submitted" {
		t.Errorf("Expected body 'submitted', got '%s'", rec.Body.String())
	}

	// 3. Failed POST with a missing token
	form = url.Values{}
	req = httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(csrfCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status BadRequest (400) on POST with missing token, got %d", rec.Code)
	}
}

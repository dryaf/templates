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

type Person struct {
	Name string
	Age  int
}

// TestMain creates a local symlink to the project's template files before running tests,
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

// setup creates and initializes an echo.Renderer and echo.Echo instance for testing.
func setup(t *testing.T) (*echoRenderer, *echo.Echo) {
	tmpls := templates.New(nil, nil)
	tmpls.MustParseTemplates()

	e := echo.New()
	renderer := Renderer(tmpls).(*echoRenderer)
	e.Renderer = renderer

	return renderer, e
}

func TestNewTemplatesRendererAndRenderer(t *testing.T) {
	t.Run("NewTemplatesRenderer", func(t *testing.T) {
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

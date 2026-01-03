// ==== File: templates_test.go ====
package templates

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/safehtml/template"
)

type Person struct {
	Name string
	Age  int64
}

//go:embed files/templates
var embededTemplates embed.FS

func failOnErr(t *testing.T, err error) {
	if err != nil {
		t.Helper()
		t.Fatalf("Error: %v", err)
	}
}

// TestRendering runs the main suite of rendering tests against different filesystem setups.
func TestRendering(t *testing.T) {
	setups := []struct {
		name  string
		setup func(t *testing.T) *Templates
	}{
		{
			"LocalFS",
			func(t *testing.T) *Templates {
				tmpls := New()
				tmpls.MustParseTemplates()
				return tmpls
			},
		},
		{
			"EmbeddedFS",
			func(t *testing.T) *Templates {
				tmpls := New(WithFileSystem(&embededTemplates))
				tmpls.MustParseTemplates()
				return tmpls
			},
		},
		{
			"DisabledSafeHTML",
			func(t *testing.T) *Templates {
				tmpls := New(WithDisableSafeHTML(true))
				tmpls.MustParseTemplates()
				return tmpls
			},
		},
	}

	for _, setup := range setups {
		t.Run(setup.name, func(t *testing.T) {
			tmpls := setup.setup(t)

			t.Run("DefaultLayoutWithPerson", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "person", &Person{Name: "Bob", Age: 39})
				failOnErr(t, err)

				if !strings.Contains(res, "Layout") ||
					!strings.Contains(res, "Person-Page") ||
					!strings.Contains(res, "Name: Bob") ||
					!strings.Contains(res, "Age: 39") {
					t.Error(res)
					t.Error("test failed, maybe layout was rendered ")
				}
			})

			t.Run("DefaultLayout", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "sample_page", "test")
				failOnErr(t, err)

				if strings.Contains(res, "Special-Layout:test") ||
					!strings.Contains(res, "Sample-Page:test") ||
					!strings.Contains(res, "Sample-Block:via_block") ||
					!strings.Contains(res, "Sample-Block-Locals:1 2 3") ||
					!strings.Contains(res, "Sample-Block-Locals:x y z") ||
					!strings.Contains(res, "Sample-Block:via_d_block") {
					t.Error(res)
					t.Error("test railed, maybe layout was rendered ")
				}
			})

			t.Run("TrustedHTML", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "trusted_html", "<b>test</b>")
				failOnErr(t, err)

				if !strings.Contains(res, "<b>test</b>") {
					t.Error(res)
					t.Error("test failed")
				}
			})

			t.Run("TrustedScript", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "trusted_script_page", "alert('hello')")
				failOnErr(t, err)
				expected := "<script>alert('hello')</script>"
				if !strings.Contains(res, expected) {
					t.Errorf("Expected to contain %q, got %q", expected, res)
				}
			})

			t.Run("TrustedStyle", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "trusted_style_page", "width: 10px;")
				failOnErr(t, err)
				expected := `<div style="width: 10px;">...</div>`
				if !strings.Contains(res, expected) {
					t.Errorf("Expected to contain %q, got %q", expected, res)
				}
			})

			t.Run("TrustedStyleSheet", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "trusted_stylesheet_page", "body { color: red; }")
				failOnErr(t, err)
				expected := "<style>body { color: red; }</style>"
				if !strings.Contains(res, expected) {
					t.Errorf("Expected to contain %q, got %q", expected, res)
				}
			})

			t.Run("TrustedURL", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "trusted_url_page", "http://example.com?a=b&c=d")
				failOnErr(t, err)

				var expected string
				if tmpls.DisableSafeHTML {
					// text/template is literal
					expected = `<a href="http://example.com?a=b&c=d">link</a>`
				} else {
					// safehtml escapes attribute values
					expected = `<a href="http://example.com?a=b&amp;c=d">link</a>`
				}

				if !strings.Contains(res, expected) {
					t.Errorf("Expected to contain %q, got %q", expected, res)
				}
			})

			t.Run("TrustedURL_Javascript", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "trusted_url_page", "javascript:alert(1)")
				failOnErr(t, err)
				// Both engines URL-encode parentheses in hrefs when safe mode is ON.
				// In prototyping mode (text/template), it is literal.
				var expected string
				if tmpls.DisableSafeHTML {
					expected = `<a href="javascript:alert(1)">link</a>`
				} else {
					expected = `<a href="javascript:alert%281%29">link</a>`
				}
				if !strings.Contains(res, expected) {
					t.Errorf("Expected to contain %q, got %q", expected, res)
				}
			})

			t.Run("TrustedResourceURL", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "trusted_resource_url_page", "/foo.js")
				failOnErr(t, err)
				expected := `<script src="/foo.js"></script>`
				if !strings.Contains(res, expected) {
					t.Errorf("Expected to contain %q, got %q", expected, res)
				}
			})

			t.Run("TrustedIdentifier", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "trusted_identifier_page", "my-id")
				failOnErr(t, err)
				expected := `<div id="my-id">...</div>`
				if !strings.Contains(res, expected) {
					t.Errorf("Expected to contain %q, got %q", expected, res)
				}
			})

			t.Run("Layout", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "special:sample_page", "test")
				failOnErr(t, err)
				if !strings.Contains(res, "Special-Layout:test") ||
					!strings.Contains(res, "Sample-Page:test") ||
					!strings.Contains(res, "Sample-Block:via_block") ||
					!strings.Contains(res, "Sample-Block:via_d_block") {
					t.Error(res)
					t.Error("Didn't contain strings ")
				}
			})

			t.Run("render_page_only", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, ":sample_page", "test")
				failOnErr(t, err)
				if strings.Contains(res, "Layout-Full:test") == false &&
					strings.Contains(res, "Sample-Page:test") &&
					strings.Contains(res, "Sample-Block:via_block") {
					t.Log("ok")
				} else {
					t.Error(res)
					t.Error("Didn't just render samp")
				}
			})

			t.Run("RenderBlockAsHTMLString", func(t *testing.T) {
				res, err := tmpls.RenderBlockAsHTMLString("_sample_block", "test")
				if err != nil {
					t.Error(err)
				}
				resStr := fmt.Sprint(res)
				if !strings.Contains(resStr, "Sample-Block:test") || strings.Contains(resStr, "should-be-hidden") {
					t.Error("err:", err)
					t.Error("res:", res)
					t.Error("Didn't contain", "Layout-Full:test")
				}
			})

			t.Run("RenderBlockAsHTMLString_Errors", func(t *testing.T) {
				_, err := tmpls.RenderBlockAsHTMLString("not_starting_with_underscore", "test")
				if !errors.Is(err, ErrInvalidBlockName) {
					t.Errorf("Expected ErrInvalidBlockName for missing underscore prefix")
				}
				_, err = tmpls.RenderBlockAsHTMLString("_", "test")
				if !errors.Is(err, ErrBlockNotFound) {
					t.Errorf("Expected ErrBlockNotFound for non-existent block")
				}
				longName := "_" + strings.Repeat("a", 255)
				_, err = tmpls.RenderBlockAsHTMLString(longName, "test")
				if !errors.Is(err, ErrInvalidBlockName) {
					t.Errorf("Expected ErrInvalidBlockName for long block name, but got: %v", err)
				}
			})

			t.Run("block_via_ExecuteTemplate", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "_sample_block", "test")
				if err != nil {
					t.Error(err)
				}
				resStr := string(res)
				if !strings.Contains(resStr, "Sample-Block:test") ||
					strings.Contains(resStr, "should-be-hidden") ||
					strings.Contains(resStr, "Page") ||
					strings.Contains(resStr, "Layout") {
					t.Error("err:", err)
					t.Error("res:", res)
					t.Error("Didn't contain", "Layout-Full:test")
				}
			})

			t.Run("block_in_block_ExecuteTemplate", func(t *testing.T) {
				res, err := tmpls.ExecuteTemplateAsText(nil, "nested", "test")
				if err != nil {
					t.Error(err)
				}
				resStr := string(res)
				if strings.Count(resStr, "should-be-hidden") != 0 ||
					strings.Count(resStr, "Layout-Full:test") != 1 ||
					strings.Count(resStr, "Level Nested:test") != 1 ||
					strings.Count(resStr, "BB:test") != 2 ||
					strings.Count(resStr, "Sample-Block:test") != 3 {
					t.Error("err:", err)
					t.Error("resStr:", resStr)
					t.Error("Didn't contain ...")
				}
			})

			t.Run("ExecuteTemplate_EmptyName", func(t *testing.T) {
				_, err := tmpls.ExecuteTemplateAsText(nil, "", "test")
				if !errors.Is(err, ErrTemplateNotFound) {
					t.Errorf("Expected ErrTemplateNotFound for empty template name, but got: %v", err)
				}
			})

			t.Run("ExecuteTemplate_LayoutFromContext", func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				ctx := context.WithValue(req.Context(), LayoutContextKey{}, "special")
				req = req.WithContext(ctx)

				res, err := tmpls.ExecuteTemplateAsText(req, "sample_page", "from_context")
				failOnErr(t, err)

				if !strings.Contains(res, "Special-Layout:from_context") {
					t.Errorf("Expected special layout from context, got: %s", res)
				}
			})

			t.Run("Templates_NotFound", func(t *testing.T) {
				_, err := tmpls.ExecuteTemplateAsText(nil, "_not_found", "test")
				if !errors.Is(err, ErrTemplateNotFound) {
					t.Errorf("Expected ErrTemplateNotFound for not found block")
				}
				_, err = tmpls.ExecuteTemplateAsText(nil, ":not_found", "test")
				if !errors.Is(err, ErrTemplateNotFound) {
					t.Errorf("Expected ErrTemplateNotFound for not found page")
				}
				_, err = tmpls.ExecuteTemplateAsText(nil, "not_found", "test")
				if !errors.Is(err, ErrTemplateNotFound) {
					t.Errorf("Expected ErrTemplateNotFound for not found page with default layout")
				}
				_, err = tmpls.ExecuteTemplateAsText(nil, "not_found:sample_page", "test")
				if !errors.Is(err, ErrTemplateNotFound) {
					t.Errorf("Expected ErrTemplateNotFound for not found layout")
				}
			})

			t.Run("GetParsedTemplates", func(t *testing.T) {
				keys := tmpls.GetParsedTemplates()
				// Based on the number of pages, layouts, and blocks
				// 10 pages * 2 layouts = 20
				// 10 pages (no layout) = 10
				// 3 blocks = 3
				// Total = 33
				expectedCount := 33
				if len(keys) != expectedCount {
					t.Errorf("Expected %d parsed templates, but got %d: %v", expectedCount, len(keys), keys)
				}
				found := false
				for _, k := range keys {
					if k == "application:person" {
						found = true
						break
					}
				}
				if !found {
					t.Error("Expected to find 'application:person' in parsed templates")
				}
			})

			t.Run("Handlers_Error", func(t *testing.T) {
				logBuf := new(bytes.Buffer)
				tmpls := setup.setup(t)
				tmpls.Logger = slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{}))

				t.Run("HandlerRenderWithData", func(t *testing.T) {
					logBuf.Reset()
					handler := tmpls.HandlerRenderWithData("nonexistent", nil)
					req := httptest.NewRequest(http.MethodGet, "/", nil)
					w := httptest.NewRecorder()
					handler(w, req)

					if !strings.Contains(logBuf.String(), "failed to execute template") {
						t.Error("Expected log message on handler error")
					}
				})

				t.Run("HandlerRenderWithDataFromContext", func(t *testing.T) {
					logBuf.Reset()
					type key string
					handler := tmpls.HandlerRenderWithDataFromContext("nonexistent", key("data"))
					req := httptest.NewRequest(http.MethodGet, "/", nil)
					w := httptest.NewRecorder()
					handler(w, req)

					if !strings.Contains(logBuf.String(), "failed to execute template") {
						t.Error("Expected log message on handler error")
					}
				})
			})
		})
	}

	// This specific test needs its own setup, so it's outside the loop.
	t.Run("ExecuteTemplate_WithReload", func(t *testing.T) {
		tmpls := New()
		tmpls.AlwaysReloadAndParseTemplates = true
		tmpls.MustParseTemplates()

		res, err := tmpls.ExecuteTemplateAsText(nil, "person", &Person{Name: "Reload", Age: 99})
		failOnErr(t, err)

		if !strings.Contains(res, "Name: Reload") {
			t.Error("Expected to render with AlwaysReloadAndParseTemplates enabled")
		}
	})
}

func TestTrustedFunctionsPassthrough(t *testing.T) {
	tmpls := New(WithDisableSafeHTML(true))

	testCases := []struct {
		name     string
		funcName string
	}{
		{"trusted_html", "trusted_html"},
		{"trusted_script", "trusted_script"},
		{"trusted_style", "trusted_style"},
		{"trusted_stylesheet", "trusted_stylesheet"},
		{"trusted_url", "trusted_url"},
		{"trusted_resource_url", "trusted_resource_url"},
		{"trusted_identifier", "trusted_identifier"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fn, ok := tmpls.funcMap[tc.funcName]
			if !ok {
				t.Fatalf("Function %s not found in funcMap", tc.funcName)
			}

			// Signature is func(any) any
			f := reflect.ValueOf(fn)
			input := reflect.ValueOf("<b>raw input</b>")

			results := f.Call([]reflect.Value{input})
			output := results[0].Interface()

			if output != "<b>raw input</b>" {
				t.Errorf("Expected output to be same as input %q, but got %v", "<b>raw input</b>", output)
			}
		})
	}

	t.Run("context aware helpers", func(t *testing.T) {
		ctx := context.Background()
		testCases := []string{
			"trusted_html_ctx",
			"trusted_script_ctx",
			"trusted_style_ctx",
			"trusted_stylesheet_ctx",
			"trusted_url_ctx",
			"trusted_resource_url_ctx",
			"trusted_identifier_ctx",
		}

		for _, name := range testCases {
			fn, ok := tmpls.funcMap[name]
			if !ok {
				t.Fatalf("Function %s not found in funcMap", name)
			}

			f := reflect.ValueOf(fn)
			inputData := reflect.ValueOf("test data")
			inputCtx := reflect.ValueOf(ctx)

			results := f.Call([]reflect.Value{inputCtx, inputData})
			output := results[0].Interface()

			if output != "test data" {
				t.Errorf("Expected output to be same as input %q, but got %v", "test data", output)
			}
		}
	})
}

func TestDisableTrustedLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	t.Run("logging enabled (default)", func(t *testing.T) {
		buf.Reset()
		tmpls := New(WithLogger(logger))
		tmpls.MustParseTemplates()

		ctx := context.Background()
		// Get helper from FuncMap and call via reflect
		fn := reflect.ValueOf(tmpls.funcMap["trusted_html_ctx"])
		fn.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf("<b>data</b>")})

		if !strings.Contains(buf.String(), "trusted_html_ctx called") {
			t.Error("Expected INFO log when logging is enabled")
		}
	})

	t.Run("logging disabled", func(t *testing.T) {
		buf.Reset()
		tmpls := New(WithLogger(logger), WithDisableTrustedLog(true))
		tmpls.MustParseTemplates()

		ctx := context.Background()
		fn := reflect.ValueOf(tmpls.funcMap["trusted_html_ctx"])
		fn.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf("<b>data</b>")})

		if buf.Len() > 0 {
			t.Errorf("Expected no logs, but got: %s", buf.String())
		}
	})

	t.Run("logging disabled in prototyping mode", func(t *testing.T) {
		buf.Reset()
		tmpls := New(WithLogger(logger), WithDisableSafeHTML(true), WithDisableTrustedLog(true))
		tmpls.MustParseTemplates()

		ctx := context.Background()
		fn := reflect.ValueOf(tmpls.funcMap["trusted_html_ctx"])
		fn.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf("<b>data</b>")})

		if buf.Len() > 0 {
			t.Errorf("Expected no logs in prototyping mode, but got: %s", buf.String())
		}
	})
}

// --- Standalone tests that don't depend on a full template engine instance ---

// unsupportedFS is a dummy fs.FS implementation for testing panics.
type unsupportedFS struct{}

func (u *unsupportedFS) Open(name string) (fs.File, error) {
	return nil, fs.ErrInvalid
}

func TestErrorsAndPanics(t *testing.T) {
	t.Run("New_UnsupportedFS", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a panic but did not get one")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("expected panic message to be a string, got %T", r)
			}
			if !strings.Contains(msg, "provided fsys is not an *embed.FS or nil") {
				t.Errorf("expected panic message to contain 'provided fsys is not an *embed.FS or nil', but got %q", msg)
			}
		}()
		New(WithFileSystem(&unsupportedFS{}))
	})

	t.Run("New_BadEmbedFS", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected a panic but did not get one")
			}
		}()
		// This should succeed, but the returned filesystem is empty.
		var badFS embed.FS
		tmpls := New(WithFileSystem(&badFS))
		tmpls.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		// This should panic because the required directories are not in the empty FS.
		tmpls.MustParseTemplates()
	})

	t.Run("MustParseTemplates_Panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected a panic but did not get one")
			}
		}()
		tmpls := New()
		tmpls.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		// Point to a non-existent directory to force a parsing error
		tmpls.fileSystem = os.DirFS("non-existent-dir")
		tmpls.MustParseTemplates() // This should panic
	})

	t.Run("DuplicateFuncMapPanics", func(t *testing.T) {
		testCases := []struct {
			name  string
			setup func(fm template.FuncMap)
		}{
			{"d_block", func(fm template.FuncMap) { fm["d_block"] = func() {} }},
			{"trusted_html", func(fm template.FuncMap) { fm["trusted_html"] = func() {} }},
			{"locals", func(fm template.FuncMap) { fm["locals"] = func() {} }},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Expected panic for duplicate function %q", tc.name)
					}
				}()
				tmpls := New(WithFuncMap(template.FuncMap{}))
				tmpls.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
				tc.setup(tmpls.funcMap)
				tmpls.AddFuncMapHelpers() // This should panic
			})
		}
	})

	t.Run("ParseTemplates_SyntaxError", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "syntax-error")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		// Create a valid structure but with a broken template file
		if err := os.MkdirAll(filepath.Join(dir, "layouts"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "pages"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(filepath.Join(dir, "layouts/app.gohtml"), []byte(`{{define "layout"}}{{block "page" .}}{{end}}{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(filepath.Join(dir, "pages/broken.gohtml"), []byte(`{{define "page"}}{{if}}{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}

		// Use a chdir hack to make the relative paths work for the test
		wd, _ := os.Getwd()
		if err := os.Chdir(filepath.Dir(dir)); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(wd)

		tmpls := New()
		// The templates path is now relative to the temp dir parent
		tmpls.fileSystem = os.DirFS(filepath.Base(dir))
		err = tmpls.ParseTemplates()
		if err == nil {
			t.Fatal("Expected a syntax error during parsing, but got nil")
		}
	})

	t.Run("ExecuteTemplate_ReloadError", func(t *testing.T) {
		tmpfile, err := ioutil.TempFile("", "reload-*.gohtml")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		// We need to create the full directory structure for New(nil,nil) to work
		dir := filepath.Dir(tmpfile.Name())
		pageDir := filepath.Join(dir, "files/templates/pages")
		layoutDir := filepath.Join(dir, "files/templates/layouts")
		blockDir := filepath.Join(dir, "files/templates/blocks")
		os.MkdirAll(pageDir, 0755)
		os.MkdirAll(layoutDir, 0755)
		os.MkdirAll(blockDir, 0755)
		defer os.RemoveAll(filepath.Join(dir, "files"))

		// Write initial valid files
		pagePath := filepath.Join(pageDir, "reload.gohtml")
		layoutPath := filepath.Join(layoutDir, "application.gohtml")
		if err := ioutil.WriteFile(pagePath, []byte(`{{define "page"}}OK{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(layoutPath, []byte(`{{define "layout"}}{{block "page" .}}{{end}}{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}

		wd, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(wd)

		tmpls := New()
		tmpls.AlwaysReloadAndParseTemplates = true
		tmpls.MustParseTemplates()

		// First render should succeed
		_, err = tmpls.ExecuteTemplateAsText(nil, "reload", nil)
		if err != nil {
			t.Fatalf("Initial render failed: %v", err)
		}

		// Now, corrupt the template
		if err := ioutil.WriteFile(pagePath, []byte(`{{define "page"}}{{if}}{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}

		// Second render should fail
		_, err = tmpls.ExecuteTemplateAsText(nil, "reload", nil)
		if err == nil {
			t.Fatal("Expected error on second render but got nil")
		}
	})

	t.Run("RenderBlockAsHTMLString_ExecutionError", func(t *testing.T) {
		tmpls := New()
		tmpls.funcMap = template.FuncMap{} // Ensure no unexpected funcs
		tmpls.templates = make(map[string]any)

		// Create a block that will fail on execution (indexing a nil map)
		// but is valid at parse time.
		tpl, err := template.New("_bad_block").Parse(`{{define "_bad_block"}}{{index . "foo"}}{{end}}`)
		if err != nil {
			t.Fatalf("Failed to parse bad block template: %v", err)
		}
		tmpls.templates["_bad_block"] = tpl

		_, err = tmpls.RenderBlockAsHTMLString("_bad_block", nil)
		if err == nil {
			t.Fatal("Expected an execution error but got nil")
		}
		if !strings.Contains(err.Error(), "index of untyped nil") {
			t.Errorf("Expected error about nil index, got: %v", err)
		}
	})
}

func Test_Locals(t *testing.T) {
	a := Locals("a", "a1", "b", 2, "c", 23.23)
	if a["a"] != "a1" {
		t.Error(a)
	}
	if a["b"] != 2 {
		t.Error(a)
	}
	if a["c"] != 23.23 {
		t.Error(a)
	}
}

func Test_AddFuncMapHelpers_Disabled(t *testing.T) {
	tpls := New()
	// Reset the map and disable the helpers to test the conditional logic in AddFuncMapHelpers
	tpls.funcMap = make(template.FuncMap)
	tpls.AddHeadlessCMSFuncMapHelpers = false
	tpls.AddFuncMapHelpers()

	if _, ok := tpls.funcMap["d_block"]; ok {
		t.Error(`d_block helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["locals"]; ok {
		t.Error(`locals helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["trusted_html"]; ok {
		t.Error(`trusted_html helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["trusted_script"]; ok {
		t.Error(`trusted_script helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["trusted_style"]; ok {
		t.Error(`trusted_style helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["trusted_stylesheet"]; ok {
		t.Error(`trusted_stylesheet helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["trusted_url"]; ok {
		t.Error(`trusted_url helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["trusted_resource_url"]; ok {
		t.Error(`trusted_resource_url helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["trusted_identifier"]; ok {
		t.Error(`trusted_identifier helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
}

func Test_trustedConverters_nil(t *testing.T) {
	if trustedHTML(nil).String() != "" {
		t.Errorf(`Expected trustedHTML(nil) to return an empty safehtml.HTML, but got %q`, trustedHTML(nil).String())
	}
	if trustedScript(nil).String() != "" {
		t.Errorf(`Expected trustedScript(nil) to return an empty safehtml.Script, but got %q`, trustedScript(nil).String())
	}
	if trustedStyle(nil).String() != "" {
		t.Errorf(`Expected trustedStyle(nil) to return an empty safehtml.Style, but got %q`, trustedStyle(nil).String())
	}
	if trustedStyleSheet(nil).String() != "" {
		t.Errorf(`Expected trustedStyleSheet(nil) to return an empty safehtml.StyleSheet, but got %q`, trustedStyleSheet(nil).String())
	}
	if trustedURL(nil).String() != "" {
		t.Errorf(`Expected trustedURL(nil) to return an empty safehtml.URL, but got %q`, trustedURL(nil).String())
	}
	if trustedResourceURL(nil).String() != "" {
		t.Errorf(`Expected trustedResourceURL(nil) to return an empty safehtml.TrustedResourceURL, but got %q`, trustedResourceURL(nil).String())
	}
	if trustedIdentifier(nil).String() != "" {
		t.Errorf(`Expected trustedIdentifier(nil) to return an empty safehtml.Identifier, but got %q`, trustedIdentifier(nil).String())
	}
}

func Test_parseNewTemplateWithFuncMap_NoFiles(t *testing.T) {
	tmpls := New() // Need an instance to get fileSystemTrusted
	_, err := tmpls.parseNewTemplateWithFuncMap("test", nil, tmpls.fileSystemTrusted)
	if err == nil || err.Error() != "no files in slice" {
		t.Errorf(`Expected error for no files, but got: %v`, err)
	}
}

func Test_cleanPath(t *testing.T) {
	tests := map[string]string{
		"":              "/",
		"a":             "a",
		"a/b":           "a/b",
		"a//b":          "a/b",
		"a/b/.":         "a/b",
		"a/b/..":        "a",
		"a/b/../c":      "a/c",
		"/a/b/..":       "/a",
		"/a/b/../../..": "/",
		"a/":            "a/",
		"/a/":           "/a/",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			if got := cleanPath(in); got != want {
				t.Errorf("cleanPath(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// --- Mocks for testing error paths ---

// mockErrorFS now correctly implements fs.FS
type mockErrorFS struct {
	openErr    error
	readdirErr error
}

func (mfs *mockErrorFS) Open(name string) (fs.File, error) {
	if mfs.openErr != nil {
		return nil, mfs.openErr
	}
	// Return a file that will error on Readdir
	return &mockErrorFile{readdirErr: mfs.readdirErr}, nil
}

// mockErrorFile now correctly implements fs.File and http.File
type mockErrorFile struct {
	readdirErr error
	fs.File
}

func (mef *mockErrorFile) Stat() (fs.FileInfo, error) { return &mockFileInfo{isDir: true}, nil }
func (mef *mockErrorFile) Read([]byte) (int, error)   { return 0, io.EOF }
func (mef *mockErrorFile) Close() error               { return nil }

// Readdir is for the http.File interface
func (mef *mockErrorFile) Readdir(count int) ([]fs.FileInfo, error) {
	if mef.readdirErr != nil {
		return nil, mef.readdirErr
	}
	return []fs.FileInfo{}, nil
}

// ReadDir is for the fs.ReadDirFile interface (part of fs.File)
func (mef *mockErrorFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if mef.readdirErr != nil {
		return nil, mef.readdirErr
	}
	return []fs.DirEntry{}, nil
}

type mockFileInfo struct {
	isDir bool
}

func (mfi *mockFileInfo) Name() string       { return "mock" }
func (mfi *mockFileInfo) Size() int64        { return 0 }
func (mfi *mockFileInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (mfi *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (mfi *mockFileInfo) IsDir() bool        { return mfi.isDir }
func (mfi *mockFileInfo) Sys() interface{}   { return nil }

func TestParseTemplatesErrors(t *testing.T) {
	symlinkTargetDir, err := ioutil.TempDir("", "templates-test-target-*")
	if err != nil {
		t.Fatalf("Failed to create symlink target dir: %v", err)
	}
	defer os.RemoveAll(symlinkTargetDir)

	const symlinkName = "files"
	const backupName = "files.bak"

	if _, err := os.Stat(symlinkName); err == nil {
		if err := os.Rename(symlinkName, backupName); err != nil {
			t.Fatalf("Failed to rename original 'files' directory: %v", err)
		}
		defer os.Rename(backupName, symlinkName)
	}

	if err := os.Symlink(symlinkTargetDir, symlinkName); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}
	defer os.Remove(symlinkName)

	t.Run("missing layouts folder", func(t *testing.T) {
		templatesRoot := filepath.Join(symlinkTargetDir, "templates")
		if err := os.MkdirAll(templatesRoot, 0755); err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(templatesRoot)
		tmpls := New()
		err := tmpls.ParseTemplates()
		if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
			t.Errorf("Expected an error for missing layouts folder, but got: %v", err)
		}
	})

	t.Run("no layouts in layouts folder", func(t *testing.T) {
		templatesRoot := filepath.Join(symlinkTargetDir, "templates")
		defer os.RemoveAll(templatesRoot)
		if err := os.MkdirAll(filepath.Join(templatesRoot, "layouts"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(templatesRoot, "pages"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(templatesRoot, "blocks"), 0755); err != nil {
			t.Fatal(err)
		}
		tmpls := New()
		err := tmpls.ParseTemplates()
		if err == nil || err.Error() != "you need at least one layout" {
			t.Errorf("Expected error for no layouts, but got: %v", err)
		}
	})

	t.Run("block name mismatch", func(t *testing.T) {
		templatesRoot := filepath.Join(symlinkTargetDir, "templates")
		defer os.RemoveAll(templatesRoot)
		if err := os.MkdirAll(filepath.Join(templatesRoot, "layouts"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(templatesRoot, "pages"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(templatesRoot, "blocks"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(filepath.Join(templatesRoot, "layouts", "app.gohtml"), []byte(`{{define "layout"}}{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(filepath.Join(templatesRoot, "blocks", "mismatch.gohtml"), []byte(`{{define "_actual"}}...{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
		tmpls := New()
		err := tmpls.ParseTemplates()
		if err == nil || !strings.Contains(err.Error(), "filename doesn't match a definition") {
			t.Errorf("Expected block name mismatch error, but got: %v", err)
		}
	})

	t.Run("duplicate block definition", func(t *testing.T) {
		templatesRoot := filepath.Join(symlinkTargetDir, "templates")
		defer os.RemoveAll(templatesRoot)

		if err := os.MkdirAll(filepath.Join(templatesRoot, "layouts"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(templatesRoot, "pages"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(templatesRoot, "blocks"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(filepath.Join(templatesRoot, "layouts", "app.gohtml"), []byte(`{{define "layout"}}{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(filepath.Join(templatesRoot, "blocks", "_dup.gohtml"), []byte(`{{define "_dup"}}...{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(filepath.Join(templatesRoot, "blocks", "dup.gohtml"), []byte(`{{define "_dup"}}...{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}

		tmpls := New()
		err := tmpls.ParseTemplates()
		if err == nil || !strings.Contains(err.Error(), "block already defined as key") {
			t.Errorf("Expected 'block already defined' error, but got: %v", err)
		}
	})

	t.Run("getFilePathsInDir readdir error", func(t *testing.T) {
		tmpls := New()
		// This mock now correctly implements fs.FS
		mockFS := &mockErrorFS{readdirErr: errors.New("forced readdir error")}
		// The internal `fileSystem` field is an fs.FS, but getFilePathsInDir takes an http.FileSystem
		// So we must convert our mock for the test.
		tmpls.fileSystem = mockFS
		err := tmpls.ParseTemplates()
		if err == nil || !strings.Contains(err.Error(), "forced readdir error") {
			t.Errorf("Expected readdir error, but got: %v", err)
		}
	})
}

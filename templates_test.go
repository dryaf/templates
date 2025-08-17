package templates

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"io"
	"io/fs"
	"io/ioutil"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
				tmpls := New(nil, nil)
				tmpls.MustParseTemplates()
				return tmpls
			},
		},
		{
			"EmbeddedFS",
			func(t *testing.T) *Templates {
				tmpls := New(&embededTemplates, nil)
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
					t.Error("test railed, maybe layout was rendered ")
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
				resStr := res.String()
				if !strings.Contains(resStr, "Sample-Block:test") || strings.Contains(resStr, "should-be-hidden") {
					t.Error("err:", err)
					t.Error("res:", res)
					t.Error("Didn't contain", "Layout-Full:test")
				}
			})

			t.Run("RenderBlockAsHTMLString_Errors", func(t *testing.T) {
				_, err := tmpls.RenderBlockAsHTMLString("not_starting_with_underscore", "test")
				if err == nil || !strings.Contains(err.Error(), "blockname needs to start with _") {
					t.Errorf("Expected error for missing underscore prefix")
				}
				_, err = tmpls.RenderBlockAsHTMLString("_", "test")
				if err == nil || !strings.Contains(err.Error(), "not found in templates-map") {
					t.Errorf("Expected error for non-existent block")
				}
				longName := "_" + strings.Repeat("a", 255)
				_, err = tmpls.RenderBlockAsHTMLString(longName, "test")
				if err == nil || !strings.Contains(err.Error(), "number of characters in string must not exceed 255") {
					t.Errorf("Expected error for long block name, but got: %v", err)
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
				if err == nil || !strings.Contains(err.Error(), "template: name not found ->application:error") {
					t.Errorf("Expected error for empty template name, but got: %v", err)
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
				if err == nil || !strings.Contains(err.Error(), "template: name not found") {
					t.Errorf("Expected error for not found block")
				}
				_, err = tmpls.ExecuteTemplateAsText(nil, ":not_found", "test")
				if err == nil || !strings.Contains(err.Error(), "template: name not found") {
					t.Errorf("Expected error for not found page")
				}
				_, err = tmpls.ExecuteTemplateAsText(nil, "not_found", "test")
				if err == nil || !strings.Contains(err.Error(), "template: name not found") {
					t.Errorf("Expected error for not found page with default layout")
				}
				_, err = tmpls.ExecuteTemplateAsText(nil, "not_found:sample_page", "test")
				if err == nil || !strings.Contains(err.Error(), "template: name not found") {
					t.Errorf("Expected error for not found layout")
				}
			})

			t.Run("GetParsedTemplates", func(t *testing.T) {
				keys := tmpls.GetParsedTemplates()
				expectedCount := 15 // Based on the number of pages, layouts, and blocks
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
				originalLogger := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{})))
				defer slog.SetDefault(originalLogger)

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
		tmpls := New(nil, nil)
		tmpls.AlwaysReloadAndParseTemplates = true
		tmpls.MustParseTemplates()

		res, err := tmpls.ExecuteTemplateAsText(nil, "person", &Person{Name: "Reload", Age: 99})
		failOnErr(t, err)

		if !strings.Contains(res, "Name: Reload") {
			t.Error("Expected to render with AlwaysReloadAndParseTemplates enabled")
		}
	})
}

// --- Standalone tests that don't depend on a full template setup ---

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
	tpls := New(nil, nil)
	// Reset the map and disable the helpers to test the conditional logic in AddFuncMapHelpers
	tpls.funcMap = make(template.FuncMap)
	tpls.AddHeadlessCMSFuncMapHelpers = false
	tpls.AddFuncMapHelpers()

	if _, ok := tpls.funcMap["d_block"]; ok {
		t.Error(`d_block helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["trusted_html"]; ok {
		t.Error(`trusted_html helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
	if _, ok := tpls.funcMap["locals"]; ok {
		t.Error(`locals helper should not exist if AddHeadlessCMSFuncMapHelpers is false`)
	}
}

func Test_trustedHTML_nil(t *testing.T) {
	res := trustedHTML(nil)
	if res.String() != "" {
		t.Errorf(`Expected trustedHTML(nil) to return an empty safehtml.HTML, but got %q`, res.String())
	}
}

func Test_parseNewTemplateWithFuncMap_NoFiles(t *testing.T) {
	tmpls := New(nil, nil) // Need an instance to get fileSystemTrusted
	_, err := parseNewTemplateWithFuncMap("test", nil, tmpls.fileSystemTrusted)
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

type mockErrorFS struct {
	openErr    error
	readdirErr error
}

func (mfs *mockErrorFS) Open(name string) (fs.File, error) {
	if mfs.openErr != nil {
		return nil, mfs.openErr
	}
	return &mockErrorFile{readdirErr: mfs.readdirErr}, nil
}

type mockErrorFile struct {
	readdirErr error
}

func (mef *mockErrorFile) Stat() (fs.FileInfo, error) { return &mockFileInfo{isDir: true}, nil }
func (mef *mockErrorFile) Read([]byte) (int, error)   { return 0, io.EOF }
func (mef *mockErrorFile) Close() error               { return nil }
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
		tmpls := New(nil, nil)
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
		tmpls := New(nil, nil)
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
		tmpls := New(nil, nil)
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

		tmpls := New(nil, nil)
		err := tmpls.ParseTemplates()
		if err == nil || !strings.Contains(err.Error(), "block already defined as key") {
			t.Errorf("Expected 'block already defined' error, but got: %v", err)
		}
	})

	t.Run("getFilePathsInDir readdir error", func(t *testing.T) {
		tmpls := New(nil, nil)
		tmpls.fileSystem = &mockErrorFS{readdirErr: errors.New("forced readdir error")}
		err := tmpls.ParseTemplates()
		if err == nil || !strings.Contains(err.Error(), "forced readdir error") {
			t.Errorf("Expected readdir error, but got: %v", err)
		}
	})
}

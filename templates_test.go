package templates

import (
	"embed"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type Person struct {
	Name string
	Age  int64
}

//go:embed files/templates
var embededTemplates embed.FS

var tmpls *Templates

func TestMain(m *testing.M) {
	var code int = 0

	// test with locals files
	tmpls = New(nil, nil)
	tmpls.MustParseTemplates()
	code = m.Run()
	if code != 0 {
		os.Exit(code)
	}

	//test with embedded files
	tmpls = New(&embededTemplates, nil)
	tmpls.MustParseTemplates()
	code = m.Run()
	os.Exit(code)
}

func failOnErr(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
}

func Test_DefaultLayoutWithPerson(t *testing.T) {
	res, err := tmpls.ExecuteTemplateAsText(nil, "person", &Person{Name: "Bob", Age: 39})
	failOnErr(t, err)

	if !strings.Contains(res, "Layout") ||
		!strings.Contains(res, "Person-Page") ||
		!strings.Contains(res, "Name: Bob") ||
		!strings.Contains(res, "Age: 39") {
		t.Error(res)
		t.Error("test failed, maybe layout was rendered ")
	}
}

func Test_DefaultLayout(t *testing.T) {
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
}

func Test_TrustedHTML(t *testing.T) {
	res, err := tmpls.ExecuteTemplateAsText(nil, "trusted_html", "<b>test</b>")
	failOnErr(t, err)

	if !strings.Contains(res, "<b>test</b>") {
		t.Error(res)
		t.Error("test railed, maybe layout was rendered ")
	}
}

func Test_Layout(t *testing.T) {
	res, err := tmpls.ExecuteTemplateAsText(nil, "special:sample_page", "test")
	failOnErr(t, err)
	if !strings.Contains(res, "Special-Layout:test") ||
		!strings.Contains(res, "Sample-Page:test") ||
		!strings.Contains(res, "Sample-Block:via_block") ||
		!strings.Contains(res, "Sample-Block:via_d_block") {
		t.Error(res)
		t.Error("Didn't contain strings ")
	}
}

func Test_render_page_only(t *testing.T) {
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
}

func Test_RenderBlockAsHTMLString(t *testing.T) {

	// OK call
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

	// NOT ok calls
	res, err = tmpls.RenderBlockAsHTMLString("not_starting_with_underscore", "test")
	if err == nil || !strings.Contains(err.Error(), "blockname needs to start with _") {
		t.Error(err, "res: ", res)
	}

	res, err = tmpls.RenderBlockAsHTMLString("_", "test")
	if err == nil || !strings.Contains(err.Error(), "not found in templates-map") {
		t.Error(err, "res: ", res)
	}
}

func Test_block_via_ExecuteTemplate(t *testing.T) {

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
}

func Test_block_in_block_ExecuteTemplate(t *testing.T) {

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
}

func Test_Templates_NotFound(t *testing.T) {

	res, err := tmpls.ExecuteTemplateAsText(nil, "_not_found", "test")
	if err == nil || !strings.Contains(err.Error(), "template: name not found") {
		t.Error(err, "res: ", res)
	}

	res, err = tmpls.ExecuteTemplateAsText(nil, ":not_found", "test")
	if err == nil || !strings.Contains(err.Error(), "template: name not found") {
		t.Error(err, "res: ", res)
	}

	res, err = tmpls.ExecuteTemplateAsText(nil, "not_found", "test")
	if err == nil || !strings.Contains(err.Error(), "template: name not found") {
		t.Error(err, "res: ", res)
	}

	res, err = tmpls.ExecuteTemplateAsText(nil, "not_found:sample_page", "test")
	if err == nil || !strings.Contains(err.Error(), "template: name not found") {
		t.Error(err, "res: ", res)
	}

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

func Test_MustParseTemplates_MissingLayouts(t *testing.T) {
	// Setup a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "test-templates")
	if err != nil {
		t.Fatal("Failed to create temporary directory:", err)
	}
	defer os.RemoveAll(tempDir)

	// Create the required folder structure without the "layouts" folder
	err = os.MkdirAll(filepath.Join(tempDir, "files", "templates"), 0755)
	if err != nil {
		t.Fatal("Failed to create folder structure:", err)
	}

	// Create a new Templates instance and set the localFS to the temporary directory
	tmpls := New(nil, nil)
	tmpls.fileSystem = os.DirFS(filepath.Join(tempDir, "files"))

	// Call ParseTemplates and expect an error
	err = tmpls.ParseTemplates()
	if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
		t.Error("Expected an error for missing layouts folder, but got:", err)
	}
}

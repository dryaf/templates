package templates_test

import (
	"os"
	"strings"
	"testing"

	testable "github.com/dryaf/templates"
)

var tmpls *testable.Templates

func TestMain(m *testing.M) {
	tmpls = testable.New(nil, "./test_files", nil)
	tmpls.MustParseTemplates()
	exitVal := m.Run()
	os.Exit(exitVal)
}

func failOnErr(t *testing.T, err error) {
	if err != nil {
		t.Error(err)
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
	resStr := string(res)
	if !strings.Contains(resStr, "Sample-Block:test") || strings.Contains(resStr, "should-be-hidden") {
		t.Error("err:", err)
		t.Error("res:", res)
		t.Error("Didn't contain", "Layout-Full:test")
	}

	// NOT ok calls
	res, err = tmpls.RenderBlockAsHTMLString("without_prepending_underscore", "test")
	if err == nil || !strings.Contains(err.Error(), "blockname needs to start with") {
		t.Error(err, "res: ", res)
	}

	res, err = tmpls.RenderBlockAsHTMLString("_not_existing", "test")
	if err == nil || !strings.Contains(err.Error(), "template _not_existing not found") {
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
	a := testable.Locals("a", "a1", "b", 2, "c", 23.23)
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

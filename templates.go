package templates

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

type Templates struct {
	AlwaysReloadAndParseTemplates bool

	cfg *TemplatesConfig

	funcMap    template.FuncMap
	fileSystem http.FileSystem

	templates map[string]*template.Template
}

// New return new Templates with default configs and templates functions to support headless cms
func New(fileSystem http.FileSystem, templatesPathInFileSystem string, fnMap template.FuncMap) *Templates {

	if fileSystem == nil {
		fileSystem = http.FS(os.DirFS("."))
		log.Default().Println("Using local filesystem '.' to parse templates in directory " + templatesPathInFileSystem)
	}

	t := &Templates{
		cfg:        DefaultTemplatesConfig(templatesPathInFileSystem),
		funcMap:    fnMap,
		fileSystem: fileSystem,
	}

	t.AddFuncMapHelpers()

	return t
}

func (t *Templates) AddFuncMapHelpers() {
	if t.funcMap == nil {
		t.funcMap = template.FuncMap{}
	}
	if t.cfg.AddHeadlessCMSFuncMapHelpers {
		t.AddDynamicBlockToFuncMap()
		t.AddTrustHTMLToFuncMap()
		t.AddLocalsToFuncMap()
	}
}

func (t *Templates) OverrideDefaultTemplateConfig(cfg *TemplatesConfig) {
	t.cfg = cfg
	t.AddFuncMapHelpers()
	fatalOnErr(t.ParseTemplates())
}

// MustParseTemplates goes fatal if there is an error
func (t *Templates) MustParseTemplates() {
	fatalOnErr(t.ParseTemplates())
}

// ParseTemplates reads all html files and freshly compiles the templates
func (t *Templates) ParseTemplates() error {
	t.templates = make(map[string]*template.Template)
	layouts, err := getFilePathsInDir(t.fileSystem, t.cfg.LayoutsPath, t.cfg.TemplateFileExtension, false)
	if err != nil {
		return errors.Wrap(err, "reading layout files")
	}
	numberOfLayouts := len(layouts)
	pages, err := getFilePathsInDir(t.fileSystem, t.cfg.PagesPath, t.cfg.TemplateFileExtension, false)
	if err != nil {
		return errors.Wrap(err, "reading pages")
	}
	blocks, err := getFilePathsInDir(t.fileSystem, t.cfg.BlocksPath, t.cfg.TemplateFileExtension, true)
	if err != nil {
		return errors.Wrap(err, "reading shared blocks")
	}
	if numberOfLayouts == 0 {
		return errors.New("you need at least one layout")
	}

	for _, layoutFilePath := range layouts {
		for _, pageFilePath := range pages {
			files := append(blocks, pageFilePath, layoutFilePath)
			layoutFilename := filepath.Base(layoutFilePath)
			layoutName := strings.TrimSuffix(layoutFilename, path.Ext(layoutFilename))
			pageFilename := filepath.Base(pageFilePath)
			pageName := strings.TrimSuffix(pageFilename, path.Ext(pageFilename))
			newTemplate, err := parseNewTemplateWithFuncMap("", t.funcMap, t.fileSystem, files...)
			if err != nil {
				return errors.Wrap(err, pageName)
			}
			t.templates[layoutName+":"+pageName] = newTemplate // sample 'application:products' aka 'layout:pageName'
		}
	}
	// Pages block "page"
	for _, pageFilePath := range pages {
		files := append(blocks, pageFilePath)
		pageFilename := filepath.Base(pageFilePath)
		pageName := strings.TrimSuffix(pageFilename, path.Ext(pageFilename))
		newTemplate, err := parseNewTemplateWithFuncMap("", t.funcMap, t.fileSystem, files...)
		if err != nil {
			return errors.Wrap(err, pageName)
		}
		t.templates[":"+pageName] = newTemplate // sample ':products'
	}
	// Blocks with prefix '_'
	for _, snippetFilePath := range blocks {
		snippetFilename := filepath.Base(snippetFilePath)
		snippetName := strings.TrimSuffix(snippetFilename, path.Ext(snippetFilename))
		newTemplate, err := parseNewTemplateWithFuncMap("", t.funcMap, t.fileSystem, snippetFilePath)
		if err != nil {
			return errors.Wrap(err, snippetFilePath)
		}
		definedTemplate := regexp.MustCompile(`^; defined templates are: |"|, `).ReplaceAllString(newTemplate.DefinedTemplates(), "")
		if _, exists := t.templates[definedTemplate]; exists || (snippetName != definedTemplate) {
			log.Fatal("fatal error - blockfile: '", snippetFilename, "' block-definition-in-file: '", definedTemplate, "' error reason 1: block already defined as key or reason 2: the filename doesnt match the blockname within")
		}
		t.templates[snippetName] = newTemplate // sample '_grid'
	}
	return nil
}

// ExecuteTemplate renders the template specified by name (layout:page or just page)
func (t *Templates) ExecuteTemplate(w io.Writer, r *http.Request, templateName string, data interface{}) error {
	// dev mode for example
	if t.AlwaysReloadAndParseTemplates {
		if err := t.ParseTemplates(); err != nil {
			return err
		}
	}

	if templateName == "" {
		templateName = "error"
	}

	// block/snippet/partial
	if templateName[:1] == "_" {
		tmpl, ok := t.templates[templateName]
		if !ok {
			return errors.New("template: name not found ->" + templateName)
		}
		return tmpl.ExecuteTemplate(w, templateName, data) // block has template name defined, so only render that
	}
	// page only
	if templateName[:1] == ":" {
		tmpl, ok := t.templates[templateName]
		if !ok {
			return errors.New("template: name not found ->" + templateName)
		}
		return tmpl.ExecuteTemplate(w, "page", data) // render page only including its blocks (every page is defined as "page" within the file for layout combination reasons as we don't have yield)
	}

	// with layout defined in templateName
	if strings.Contains(templateName, ":") {
		tmpl, ok := t.templates[templateName]
		if !ok {
			return errors.New("template: name not found ->" + templateName)
		}
		return tmpl.ExecuteTemplate(w, "", data)
	}

	// with layout [from request-context or default from config]
	layoutIsSetInContext := false
	if r != nil {
		var layout string
		layout, layoutIsSetInContext = r.Context().Value(LayoutContextKey{}).(string)
		if layoutIsSetInContext {
			templateName = fmt.Sprint(layout, ":", templateName)
		}
	}
	if !layoutIsSetInContext {
		templateName = fmt.Sprint(t.cfg.DefaultLayout, ":", templateName)
	}

	tmpl, ok := t.templates[templateName]
	if !ok {
		return errors.New("template: name not found ->" + templateName)
	}
	return tmpl.ExecuteTemplate(w, "", data)
}

// RenderBlockAsHTMLString renders a template from the templates-map as a HTML-String
func (t *Templates) RenderBlockAsHTMLString(blockname string, payload interface{}) (template.HTML, error) {
	if blockname[:1] != "_" {
		return "", errors.New("blockname needs to start with _")
	}
	if len(blockname) > 255 {
		return "", errors.New("number of characters in string must not exceed 255")
	}
	b := bytes.Buffer{}
	tt, ok := t.templates[blockname]
	if !ok {
		return "", errors.New("template " + blockname + " not found in templates-map")
	}
	err := tt.ExecuteTemplate(&b, blockname, payload)
	return template.HTML(b.String()), err
}

// AddDynamicBlockToFuncMap adds 'd_block' to the FuncMap which allows to render blocks from variables dynamically set
// Rob doesn't like, we add it anyway because we like headless cms
// https://stackoverflow.com/questions/28830543/how-to-use-a-field-of-struct-or-variable-value-as-template-name
func (t *Templates) AddDynamicBlockToFuncMap() {
	_, ok := t.funcMap["d_block"]
	if ok {
		log.Fatal("d_block is already in use")
	}
	t.funcMap["d_block"] = t.RenderBlockAsHTMLString
}

func (t *Templates) AddTrustHTMLToFuncMap() {
	_, ok := t.funcMap["trust_html"]
	if ok {
		log.Fatal("trust_html is already in use")
	}
	t.funcMap["trust_html"] = trust_html
}

func (t *Templates) AddLocalsToFuncMap() {
	_, ok := t.funcMap["locals"]
	if ok {
		log.Fatal("locals is already in use")
	}
	t.funcMap["locals"] = Locals
}

// HandlerRenderWithData returns a Handler function which only renders the template
func (t *Templates) HandlerRenderWithData(templateName string, data interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := t.ExecuteTemplate(w, r, templateName, data)
		if err != nil {
			log.Println(err)
		}
	}
}

// HandlerRenderWithDataFromContext returns a Handler function which only renders the template and uses data from context
func (t *Templates) HandlerRenderWithDataFromContext(templateName string, contextKey interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := t.ExecuteTemplate(w, r, templateName, r.Context().Value(contextKey))
		if err != nil {
			log.Println(err)
		}
	}
}

// For Testing

func (t *Templates) GetParsedTemplates() []string {
	keys := make([]string, 0, len(t.templates))

	for k := range t.templates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (t *Templates) ExecuteTemplateAsText(r *http.Request, templateName string, data interface{}) (string, error) {
	b := &bytes.Buffer{}
	err := t.ExecuteTemplate(b, r, templateName, data)
	if err != nil {
		return "", err
	}
	return string(template.HTML(b.String())), nil
}

func trust_html(html any) template.HTML {
	if html == nil {
		return template.HTML("")
	}
	return template.HTML(fmt.Sprint(html))
}

func Locals(args ...any) map[string]any {
	m := map[string]any{}
	var key any
	for i, arg := range args {
		if i%2 == 0 {
			key = arg
		} else {
			m[fmt.Sprint(key)] = arg
		}
	}
	return m
}

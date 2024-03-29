package templates

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"embed"

	"log/slog"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"github.com/google/safehtml/uncheckedconversions"
)

// LayoutContextKey is the key for getting the layout string out of the context
type LayoutContextKey struct{}

const templatesPath = "files/templates"
const layoutsPath = "layouts"
const pagesPath = "pages"
const blocksPath = "blocks"
const fileExtension = ".gohtml"

type Templates struct {
	AlwaysReloadAndParseTemplates bool

	DefaultLayout         string
	TemplateFileExtension string

	TemplatesPath template.TrustedSource
	LayoutsPath   string
	PagesPath     string
	BlocksPath    string

	AddHeadlessCMSFuncMapHelpers bool

	funcMap template.FuncMap

	fileSystem        fs.FS
	fileSystemTrusted template.TrustedFS
	fileSystemIsEmbed bool

	templates     map[string]*template.Template
	templatesLock sync.RWMutex
}

// New return new Templates with default configs and templates functions to support headless cms
func New(efs *embed.FS, fnMap template.FuncMap) *Templates {

	var trustedFileSystem template.TrustedFS
	var fileSystem fs.FS
	if efs != nil {
		fsys, err := fs.Sub(*efs, templatesPath)
		if err != nil {
			panic(fmt.Errorf("unable to create file system: %w", err))
		}
		fileSystem = fsys
		trustedFileSystem = template.TrustedFSFromEmbed(*efs)
	} else {
		fileSystem = os.DirFS(templatesPath)
		trustedFileSystem = template.TrustedFSFromTrustedSource(template.TrustedSourceFromConstant(templatesPath))
	}

	t := &Templates{
		DefaultLayout:         "application",
		TemplateFileExtension: ".gohtml",
		LayoutsPath:           layoutsPath,
		PagesPath:             pagesPath,
		BlocksPath:            blocksPath,

		AddHeadlessCMSFuncMapHelpers: true, // d_block, trust_html
		funcMap:                      fnMap,

		fileSystem:        fileSystem,
		fileSystemTrusted: trustedFileSystem,
		fileSystemIsEmbed: efs != nil,
	}

	t.AddFuncMapHelpers()

	return t
}

func (t *Templates) AddFuncMapHelpers() {
	if t.funcMap == nil {
		t.funcMap = template.FuncMap{}
	}
	if t.AddHeadlessCMSFuncMapHelpers {
		t.AddDynamicBlockToFuncMap()
		t.AddTrustedHTMLToFuncMap()
		t.AddLocalsToFuncMap()
	}
}

// MustParseTemplates goes fatal if there is an error
func (t *Templates) MustParseTemplates() {
	fatalOnErr(t.ParseTemplates())
}

// ParseTemplates reads all html files and freshly compiles the templates
func (t *Templates) ParseTemplates() error {
	t.templates = make(map[string]*template.Template)
	hfs := http.FS(t.fileSystem)
	layouts, err := getFilePathsInDir(hfs, t.LayoutsPath, t.fileSystemIsEmbed)
	if err != nil {
		return fmt.Errorf("reading layout files: %w", err)
	}
	numberOfLayouts := len(layouts)
	pages, err := getFilePathsInDir(hfs, t.PagesPath, t.fileSystemIsEmbed)
	if err != nil {
		return fmt.Errorf("reading pages: %w", err)
	}
	blocks, err := getFilePathsInDir(hfs, t.BlocksPath, t.fileSystemIsEmbed)
	if err != nil {
		return fmt.Errorf("reading shared blocks: %w", err)
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
			newTemplate, err := parseNewTemplateWithFuncMap("", t.funcMap, t.fileSystemTrusted, files...)
			if err != nil {
				return fmt.Errorf("%s: %w", pageName, err)
			}
			t.templates[layoutName+":"+pageName] = newTemplate // sample 'application:products' aka 'layout:pageName'
		}
	}
	// Page   "page" + blocks
	for _, pageFilePath := range pages {
		files := append(blocks, pageFilePath) // blocks and this one page file will end up in a template
		pageFilename := filepath.Base(pageFilePath)
		pageName := strings.TrimSuffix(pageFilename, path.Ext(pageFilename))
		newTemplate, err := parseNewTemplateWithFuncMap("", t.funcMap, t.fileSystemTrusted, files...)
		if err != nil {
			return fmt.Errorf("%s: %w", pageName, err)
		}
		t.templates[":"+pageName] = newTemplate // sample ':products'
	}
	// Blocks with prefix '_'
	for _, blockFilePath := range blocks {
		blockFilename := filepath.Base(blockFilePath)
		blockName := strings.TrimSuffix(blockFilename, path.Ext(blockFilename))
		newTemplate, err := parseNewTemplateWithFuncMap("", t.funcMap, t.fileSystemTrusted, blockFilePath)
		if err != nil {
			return fmt.Errorf("%s: %w", blockFilePath, err)
		}

		prefixedBlockName := blockName
		if blockName[:1] != "_" {
			prefixedBlockName = "_" + blockName
		}

		if _, exists := t.templates[prefixedBlockName]; exists || !definedTemplatesContain(newTemplate, prefixedBlockName) {
			return fmt.Errorf("error reason 1: block already defined as key or reason 2: the filename doesn't match a definition within the file block_filename %s defined_name %s", blockFilename, blockName)
		}
		t.templates[prefixedBlockName] = newTemplate // sample '_grid'
	}
	return nil
}

func definedTemplatesContain(t *template.Template, name string) bool {
	templates := t.Templates()
	for _, tmpl := range templates {
		if tmpl.Tree == nil || tmpl.Tree.Root.Pos == 0 {
			continue
		}
		if tmpl.Name() == name {
			return true
		}
	}
	return false
}

// ExecuteTemplate renders the template specified by name (layout:page or just page)
func (t *Templates) ExecuteTemplate(w io.Writer, r *http.Request, templateName string, data interface{}) error {
	// dev mode for example
	if t.AlwaysReloadAndParseTemplates {
		if t.templatesLock.TryLock() {
			err := t.ParseTemplates()
			t.templatesLock.Unlock()
			if err != nil {
				return err
			}
		}
		t.templatesLock.RLock()
		defer t.templatesLock.RUnlock()
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
		return tmpl.ExecuteTemplate(w, "layout", data)
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
		templateName = fmt.Sprint(t.DefaultLayout, ":", templateName)
	}

	tmpl, ok := t.templates[templateName]
	if !ok {
		return errors.New("template: name not found ->" + templateName)
	}
	return tmpl.ExecuteTemplate(w, "layout", data)
}

// RenderBlockAsHTMLString renders a template from the templates-map as a HTML-String
func (t *Templates) RenderBlockAsHTMLString(blockname string, payload interface{}) (safehtml.HTML, error) {
	if blockname[:1] != "_" {
		return safehtml.HTML{}, errors.New("blockname needs to start with _")
	}
	if len(blockname) > 255 {
		return safehtml.HTML{}, errors.New("number of characters in string must not exceed 255")
	}
	b := bytes.Buffer{}
	tt, ok := t.templates[blockname]
	if !ok {
		return safehtml.HTML{}, errors.New("template " + blockname + " not found in templates-map")
	}
	err := tt.ExecuteTemplate(&b, blockname, payload)

	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(b.String()), err
}

// AddDynamicBlockToFuncMap adds 'd_block' to the FuncMap which allows to render blocks from variables dynamically set
// Rob doesn't like, we add it anyway because we like headless cms
// https://stackoverflow.com/questions/28830543/how-to-use-a-field-of-struct-or-variable-value-as-template-name
func (t *Templates) AddDynamicBlockToFuncMap() {
	_, ok := t.funcMap["d_block"]
	if ok {
		slog.Error("func_map", errors.New("d_block is already in use"))
		os.Exit(1)
	}
	t.funcMap["d_block"] = t.RenderBlockAsHTMLString
}

func (t *Templates) AddTrustedHTMLToFuncMap() {
	_, ok := t.funcMap["trusted_html"]
	if ok {
		slog.Error("func_map", errors.New("trusted_html is already in use"))
		os.Exit(1)
	}
	t.funcMap["trusted_html"] = trustedHTML
}

func (t *Templates) AddLocalsToFuncMap() {
	_, ok := t.funcMap["locals"]
	if ok {
		slog.Error("locals", errors.New("locals is already in use"))
		os.Exit(1)
	}
	t.funcMap["locals"] = Locals
}

// HandlerRenderWithData returns a Handler function which only renders the template
func (t *Templates) HandlerRenderWithData(templateName string, data interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := t.ExecuteTemplate(w, r, templateName, data)
		if err != nil {
			slog.Error("template", err)
		}
	}
}

// HandlerRenderWithDataFromContext returns a Handler function which only renders the template and uses data from context
func (t *Templates) HandlerRenderWithDataFromContext(templateName string, contextKey interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := t.ExecuteTemplate(w, r, templateName, r.Context().Value(contextKey))
		if err != nil {
			slog.Error("template", err)
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
	return b.String(), nil
}

func trustedHTML(html any) safehtml.HTML {
	if html == nil {
		return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract("")
	}
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(fmt.Sprint(html))
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

func fatalOnErr(err error) {
	if err != nil {
		slog.Error("fatal", err)
		os.Exit(1)
	}
}

func getFilePathsInDir(fs http.FileSystem, dirPath string, prefixTemplatesPath bool) ([]string, error) {

	dirPath = cleanPath(dirPath)
	dir, err := fs.Open(dirPath)
	if err != nil {
		return nil, fmt.Errorf("getFilePathsInDir fs.Open: %w", err)

	}
	fileInfos, err := dir.Readdir(-1)
	if err != nil {
		return nil, fmt.Errorf("getFilePathsInDir Readdir: %w", err)
	}
	files := []string{}
	for _, fileInfo := range fileInfos {
		if path.Ext(fileInfo.Name()) == fileExtension {
			if prefixTemplatesPath {
				files = append(files, cleanPath(filepath.Join(templatesPath, dirPath, fileInfo.Name())))
			} else {
				files = append(files, cleanPath(filepath.Join(dirPath, fileInfo.Name())))
			}
		}
	}
	return files, nil
}

func parseNewTemplateWithFuncMap(layout string, fnMap template.FuncMap, fs template.TrustedFS, files ...string) (*template.Template, error) {
	if len(files) == 0 {
		return nil, errors.New("no files in slice")
	}
	t := template.New(layout).Funcs(fnMap)

	t, err := t.ParseFS(fs, files...)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// cleanPath returns the canonical path for p, eliminating . and .. elements.
// taken from https://golang.org/src/net/http/server.go?s=68684:68715#L2203
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	// if p[0] != '/' {
	// 	p = "/" + p
	// }
	np := path.Clean(p)
	// path.Clean removes trailing slash except for root;
	// put the trailing slash back if necessary.
	if p[len(p)-1] == '/' && np != "/" {
		// Fast path for common case of p being the string we want:
		if len(p) == len(np)+1 && strings.HasPrefix(p, np) {
			np = p
		} else {
			np += "/"
		}
	}
	return np
}

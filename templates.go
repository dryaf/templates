// ==== File: templates.go ====
// Package templates provides a secure, file-system-based Go template engine
// built on Google's safehtml/template library.
//
// # Core Concepts
//
// This engine organizes templates into a specific directory structure:
//
//   - layouts/: specific wrapper templates that define the document structure (e.g. <html>...</html>).
//   - pages/: individual page templates that render inside a layout.
//   - blocks/: reusable partials (e.g. forms, navigational elements) that can be included in pages or layouts.
//
// # Safety
//
// Using safehtml/template ensures that your output is free from XSS vulnerabilities by default.
// Context-aware escaping is applied automatically. For cases where you strictly trust the input
// (e.g. from a CMS), special helper functions `trusted_*` are provided.
package templates

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"github.com/google/safehtml/uncheckedconversions"
)

// LayoutContextKey is the key used to store and retrieve the desired layout name
// from a request's context. A middleware can set this value to dynamically
// change the layout for a request.
type LayoutContextKey struct{}

const templatesPath = "files/templates"
const layoutsPath = "layouts"
const pagesPath = "pages"
const blocksPath = "blocks"
const fileExtension = ".gohtml"

// Templates is the core engine for managing, parsing, and executing templates.
// It holds the parsed templates, configuration, and the underlying filesystem.
type Templates struct {
	// If true, templates will be re-parsed on every ExecuteTemplate call.
	// This is highly recommended for development to see changes without restarting
	// the application, but should be disabled in production for performance.
	AlwaysReloadAndParseTemplates bool

	// The name of the default layout file (without extension) to use when a
	// layout is not explicitly specified in the template name.
	// Defaults to "application".
	DefaultLayout string

	// The file extension for template files. Defaults to ".gohtml".
	TemplateFileExtension string

	// The trusted source path for templates, used by the underlying safehtml/template
	// library for security checks.
	TemplatesPath template.TrustedSource

	// The subdirectory within the templates path for layout files.
	// Defaults to "layouts".
	LayoutsPath string

	// The subdirectory within the templates path for page files.
	// Defaults to "pages".
	PagesPath string

	// The subdirectory within the templates path for reusable block files.
	// Defaults to "blocks".
	BlocksPath string

	// If true, automatically adds helper functions like `d_block`, `locals`,
	// `references` and `trusted_*` to the template function map. Defaults to true.
	AddHeadlessCMSFuncMapHelpers bool

	// The logger to use for internal errors and debug messages.
	// Defaults to slog.Default().
	Logger *slog.Logger

	funcMap template.FuncMap

	fileSystem        fs.FS
	fileSystemTrusted template.TrustedFS
	fileSystemIsEmbed bool

	templates     map[string]*template.Template
	templatesLock sync.RWMutex
}

// New creates a new Templates instance from a filesystem and a custom function map.
//
// Parameters:
//   - fsys: The filesystem containing the templates. Due to security constraints
//     in `safehtml/template`, this must be either an `*embed.FS` (for production)
//     or `nil` to use the local operating system filesystem (for development).
//     Providing any other type will cause a panic.
//   - fnMap: A `template.FuncMap` containing custom functions to make available
//     within templates. Can be nil if no custom functions are needed.
//
// Returns a new, configured *Templates instance.
func New(fsys fs.FS, fnMap template.FuncMap) *Templates {
	var trustedFileSystem template.TrustedFS
	var fileSystemForParsing fs.FS
	isEmbed := false

	switch v := fsys.(type) {
	case nil:
		// Default to OS filesystem, chrooted to the templates path.
		fileSystemForParsing = os.DirFS(templatesPath)
		trustedFileSystem = template.TrustedFSFromTrustedSource(template.TrustedSourceFromConstant(templatesPath))
	case *embed.FS:
		// It's an embedded filesystem.
		sub, err := fs.Sub(v, templatesPath)
		if err != nil {
			panic(fmt.Errorf("unable to create sub-filesystem for templates: %w", err))
		}
		fileSystemForParsing = sub
		trustedFileSystem = template.TrustedFSFromEmbed(*v)
		isEmbed = true
	default:
		panic("templates.New: provided fsys is not an *embed.FS or nil. Due to security constraints in the underlying safehtml/template library, only embedded filesystems or the OS filesystem (when fsys is nil) are supported.")
	}

	t := &Templates{
		DefaultLayout:         "application",
		TemplateFileExtension: ".gohtml",
		LayoutsPath:           layoutsPath,
		PagesPath:             pagesPath,
		BlocksPath:            blocksPath,

		AddHeadlessCMSFuncMapHelpers: true, // d_block, trust_html
		Logger:                       slog.Default(),
		funcMap:                      fnMap,

		fileSystem:        fileSystemForParsing,
		fileSystemTrusted: trustedFileSystem,
		fileSystemIsEmbed: isEmbed,
	}

	t.AddFuncMapHelpers()

	return t
}

// AddFuncMapHelpers populates the template function map with the default helpers
// if `AddHeadlessCMSFuncMapHelpers` is true. It will panic if a function name
// is already in use.
func (t *Templates) AddFuncMapHelpers() {
	if t.funcMap == nil {
		t.funcMap = template.FuncMap{}
	}
	if t.AddHeadlessCMSFuncMapHelpers {
		t.AddDynamicBlockToFuncMap()
		t.addTrustedConverterFuncs()
		t.AddLocalsToFuncMap()
		t.AddReferencesToFuncMap()
	}
}

// MustParseTemplates parses all template files from the configured filesystem.
// It will panic if any error occurs during parsing, making it suitable for
// application initialization.
func (t *Templates) MustParseTemplates() {
	t.fatalOnErr(t.ParseTemplates())
}

// ParseTemplates reads and parses all template files from the configured layouts,
// pages, and blocks directories. It populates the internal template map.
// This method is safe for concurrent use.
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
		if !strings.HasPrefix(blockName, "_") {
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

// ExecuteTemplate renders a template by name to the given writer.
//
// The `templateName` parameter supports several syntaxes:
//   - "page_name": Renders the page within the default layout (or a layout from context).
//   - "layout_name:page_name": Renders the page within a specific layout.
//   - ":page_name": Renders the page without any layout.
//   - "_block_name": Renders a specific block by itself.
//
// Parameters:
//   - w: The io.Writer to write the output to (e.g., an http.ResponseWriter).
//   - r: The *http.Request for the current request. Can be nil, but if provided,
//     the engine will check its context for a LayoutContextKey to override the layout.
//   - templateName: The name of the template to execute.
//   - data: The data to pass to the template.
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
	if strings.HasPrefix(templateName, "_") {
		tmpl, ok := t.templates[templateName]
		if !ok {
			return errors.New("template: name not found ->" + templateName)
		}
		return tmpl.ExecuteTemplate(w, templateName, data) // block has template name defined, so only render that
	}
	// page only
	if strings.HasPrefix(templateName, ":") {
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

// RenderBlockAsHTMLString renders a specific block to a safehtml.HTML string.
// This is useful for rendering partials inside other logic. The block name must
// start with an underscore "_".
func (t *Templates) RenderBlockAsHTMLString(blockname string, payload interface{}) (safehtml.HTML, error) {
	if !strings.HasPrefix(blockname, "_") {
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

// AddDynamicBlockToFuncMap adds the 'd_block' function to the FuncMap. This
// powerful helper allows templates to render blocks dynamically by name, which
// is ideal for pages whose structure is determined by an API response (e.g.,
// from a headless CMS).
// Usage in template: `{{ d_block "block_name_from_variable" .Data }}`
func (t *Templates) AddDynamicBlockToFuncMap() {
	_, ok := t.funcMap["d_block"]
	if ok {
		t.Logger.Error("function name is already in use in FuncMap", "name", "d_block")
		panic("function name 'd_block' is already in use in FuncMap")
	}
	t.funcMap["d_block"] = t.RenderBlockAsHTMLString
}

// addTrustedConverterFuncs adds the 'trusted_*' functions to the FuncMap.
// These functions wrap strings in the appropriate `safehtml` types, preventing
// them from being escaped. Use these only when you are certain the content is
// from a trusted source and is safe to render verbatim.
func (t *Templates) addTrustedConverterFuncs() {
	add := func(name string, f any) {
		if _, ok := t.funcMap[name]; ok {
			t.Logger.Error("function name is already in use in FuncMap", "name", name)
			panic(fmt.Sprintf("function name %q is already in use in FuncMap", name))
		}
		t.funcMap[name] = f
	}

	add("trusted_html", trustedHTML)
	add("trusted_script", trustedScript)
	add("trusted_style", trustedStyle)
	add("trusted_stylesheet", trustedStyleSheet)
	add("trusted_url", trustedURL)
	add("trusted_resource_url", trustedResourceURL)
	add("trusted_identifier", trustedIdentifier)
}

// AddLocalsToFuncMap adds the 'locals' function to the FuncMap. This helper
// provides a convenient way to create a `map[string]any` inside a template,
// which is useful for passing structured data to blocks.
// Usage: `{{ block "_myblock" (locals "key1" "value1" "key2" 2) }}`
func (t *Templates) AddLocalsToFuncMap() {
	_, ok := t.funcMap["locals"]
	if ok {
		t.Logger.Error("function name is already in use in FuncMap", "name", "locals")
		panic("function name 'locals' is already in use in FuncMap")
	}
	t.funcMap["locals"] = Locals
}

// AddReferencesToFuncMap adds the 'references' function to the FuncMap. This helper
// provides a convenient way to create a `map[string]any` inside a template,
// where all values are pointers.
// Usage: `{{ block "_myblock" (references "key1" "value1" "key2" 2) }}`
func (t *Templates) AddReferencesToFuncMap() {
	_, ok := t.funcMap["references"]
	if ok {
		t.Logger.Error("function name is already in use in FuncMap", "name", "references")
		panic("function name 'references' is already in use in FuncMap")
	}
	t.funcMap["references"] = References
}

// HandlerRenderWithData returns a http.HandlerFunc that renders a template with
// the provided static data.
func (t *Templates) HandlerRenderWithData(templateName string, data interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := t.ExecuteTemplate(w, r, templateName, data)
		if err != nil {
			t.Logger.Error("failed to execute template", "error", err, "template_name", templateName)
		}
	}
}

// HandlerRenderWithDataFromContext returns a http.HandlerFunc that renders a
// template, taking its data from the request's context via the provided context key.
func (t *Templates) HandlerRenderWithDataFromContext(templateName string, contextKey interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := t.ExecuteTemplate(w, r, templateName, r.Context().Value(contextKey))
		if err != nil {
			t.Logger.Error("failed to execute template", "error", err, "template_name", templateName)
		}
	}
}

// GetParsedTemplates returns a sorted slice of the names of all parsed templates.
// This is primarily intended for debugging and testing purposes.
func (t *Templates) GetParsedTemplates() []string {
	keys := make([]string, 0, len(t.templates))

	for k := range t.templates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ExecuteTemplateAsText is a testing helper that renders a template to a string.
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

func trustedScript(script any) safehtml.Script {
	if script == nil {
		return uncheckedconversions.ScriptFromStringKnownToSatisfyTypeContract("")
	}
	return uncheckedconversions.ScriptFromStringKnownToSatisfyTypeContract(fmt.Sprint(script))
}

func trustedStyle(style any) safehtml.Style {
	if style == nil {
		return uncheckedconversions.StyleFromStringKnownToSatisfyTypeContract("")
	}
	return uncheckedconversions.StyleFromStringKnownToSatisfyTypeContract(fmt.Sprint(style))
}

func trustedStyleSheet(ss any) safehtml.StyleSheet {
	if ss == nil {
		return uncheckedconversions.StyleSheetFromStringKnownToSatisfyTypeContract("")
	}
	return uncheckedconversions.StyleSheetFromStringKnownToSatisfyTypeContract(fmt.Sprint(ss))
}

func trustedURL(url any) safehtml.URL {
	if url == nil {
		return uncheckedconversions.URLFromStringKnownToSatisfyTypeContract("")
	}
	return uncheckedconversions.URLFromStringKnownToSatisfyTypeContract(fmt.Sprint(url))
}

func trustedResourceURL(url any) safehtml.TrustedResourceURL {
	if url == nil {
		return uncheckedconversions.TrustedResourceURLFromStringKnownToSatisfyTypeContract("")
	}
	return uncheckedconversions.TrustedResourceURLFromStringKnownToSatisfyTypeContract(fmt.Sprint(url))
}

func trustedIdentifier(id any) safehtml.Identifier {
	if id == nil {
		return uncheckedconversions.IdentifierFromStringKnownToSatisfyTypeContract("")
	}
	return uncheckedconversions.IdentifierFromStringKnownToSatisfyTypeContract(fmt.Sprint(id))
}

// Locals is a template helper function that creates a map[string]any from a
// sequence of key-value pairs. This is useful for passing named arguments to templates.
// The arguments must be in pairs, e.g., `locals "key1", "value1", "key2", 2`.
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

// References is a template helper function that creates a map[string]any from a
// sequence of key-value pairs. Unlike Locals, it ensures that every value in
// the map is a pointer. If a value is already a pointer, it is used as is.
// If it is not a pointer, a new pointer to the value is created.
func References(args ...any) map[string]any {
	m := map[string]any{}
	var key any
	for i, arg := range args {
		if i%2 == 0 {
			key = arg
		} else {
			// Ensure we have a pointer to the value
			val := reflect.ValueOf(arg)
			var ptr any
			if !val.IsValid() {
				// Handle nil input
				ptr = nil
			} else if val.Kind() == reflect.Ptr {
				ptr = arg
			} else {
				// Create a new pointer to a copy of the value
				p := reflect.New(val.Type())
				p.Elem().Set(val)
				ptr = p.Interface()
			}
			m[fmt.Sprint(key)] = ptr
		}
	}
	return m
}

func (t *Templates) fatalOnErr(err error) {
	if err != nil {
		t.Logger.Error("fatal error during setup", "error", err)
		panic(err)
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
	var files []string
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
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	np := path.Clean(p)
	if p[len(p)-1] == '/' && np != "/" {
		if len(p) == len(np)+1 && strings.HasPrefix(p, np) {
			np = p
		} else {
			np += "/"
		}
	}
	return np
}

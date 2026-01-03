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
//
// # Rapid Prototyping and HTMX
//
// If the strictness of safehtml is an obstacle during development or for projects using
// libraries like HTMX where security models might differ, it can be disabled via
// the DisableSafeHTML configuration. In this mode, the engine falls back to the
// standard library's text/template (producing literal output), and trusted_*
// functions return objects as-is.
//
// Logging for trusted helper usage can also be disabled via DisableTrustedLog to
// reduce noise in projects that use them frequently.
package templates

import (
	"bytes"
	"context"
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

	text_template "text/template"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	template_unchecked "github.com/google/safehtml/template/uncheckedconversions"
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

var (
	// ErrTemplateNotFound is returned when a requested template or layout is not in the parsed map.
	ErrTemplateNotFound = errors.New("template not found")
	// ErrBlockNotFound is returned when a requested block is not found.
	ErrBlockNotFound = errors.New("block not found")
	// ErrInvalidBlockName is returned when a block name does not adhere to naming conventions (e.g. must start with '_').
	ErrInvalidBlockName = errors.New("invalid block name")
)

// Templates is the core engine for managing, parsing, and executing templates.
// It holds the parsed templates, configuration, and the underlying filesystem.
type Templates struct {
	// If true, templates will be re-parsed on every ExecuteTemplate call.
	// This is highly recommended for development to see changes without restarting
	// the application, but should be disabled in production for performance.
	AlwaysReloadAndParseTemplates bool

	// If true, disables the use of safehtml/template and uses standard text/template instead.
	// This is useful for rapid prototyping or for projects where safehtml is not required.
	// When true, NO auto-escaping occurs, and trusted_* functions return objects as-is.
	DisableSafeHTML bool

	// If true, suppresses the INFO log message normally generated when a
	// trusted_*_ctx helper function is used.
	DisableTrustedLog bool

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

	// Internal configuration
	rootPath string
	inputFS  fs.FS

	templates     map[string]any
	templatesLock sync.RWMutex
}

// Option defines a functional option for configuring the Templates engine.
type Option func(*Templates)

// WithFileSystem sets the filesystem and root path for templates.
// The rootPath argument specifies the directory within the filesystem where templates are stored.
// If fs is nil, it uses the OS filesystem rooted at the given path.
func WithFileSystem(fsys fs.FS) Option {
	return func(t *Templates) {
		t.inputFS = fsys
	}
}

// WithRoot sets the root path for templates.
func WithRoot(path string) Option {
	return func(t *Templates) {
		t.rootPath = path
	}
}

// WithFuncMap adds custom functions to the template engine.
func WithFuncMap(fm template.FuncMap) Option {
	return func(t *Templates) {
		t.funcMap = fm
	}
}

// WithReload enables or disables always reloading parsing templates on execution.
func WithReload(enabled bool) Option {
	return func(t *Templates) {
		t.AlwaysReloadAndParseTemplates = enabled
	}
}

// WithDisableSafeHTML enables or disables the use of safehtml/template.
// When disabled, the standard library's text/template is used for literal output.
func WithDisableSafeHTML(disabled bool) Option {
	return func(t *Templates) {
		t.DisableSafeHTML = disabled
	}
}

// WithDisableTrustedLog enables or disables INFO logs for trusted helpers.
func WithDisableTrustedLog(disabled bool) Option {
	return func(t *Templates) {
		t.DisableTrustedLog = disabled
	}
}

// WithLogger sets a custom logger.
func WithLogger(l *slog.Logger) Option {
	return func(t *Templates) {
		t.Logger = l
	}
}

// New creates a new Templates instance configured with the provided options.
//
// By default, it looks for templates in "files/templates" within the OS filesystem.
// You can change this using `WithFileSystem` or `WithRoot`.
func New(opts ...Option) *Templates {
	t := &Templates{
		DefaultLayout:         "application",
		TemplateFileExtension: ".gohtml",
		LayoutsPath:           layoutsPath,
		PagesPath:             pagesPath,
		BlocksPath:            blocksPath,

		AddHeadlessCMSFuncMapHelpers: true, // d_block, trust_html
		Logger:                       slog.Default(),
		funcMap:                      make(template.FuncMap),

		rootPath: templatesPath,
		inputFS:  nil,
	}

	for _, opt := range opts {
		opt(t)
	}

	t.initFileSystem()
	t.AddFuncMapHelpers()

	return t
}

func (t *Templates) initFileSystem() {
	if t.rootPath == "" {
		panic("templates.New: rootPath must not be empty")
	}

	var trustedFileSystem template.TrustedFS
	var fileSystemForParsing fs.FS
	isEmbed := false

	switch v := t.inputFS.(type) {
	case nil:
		// Default to OS filesystem, chrooted to the templates path.
		fileSystemForParsing = os.DirFS(t.rootPath)
		trustedFileSystem = template.TrustedFSFromTrustedSource(template_unchecked.TrustedSourceFromStringKnownToSatisfyTypeContract(t.rootPath))
	case *embed.FS:
		// It's an embedded filesystem.
		sub, err := fs.Sub(v, t.rootPath)
		if err != nil {
			panic(fmt.Errorf("unable to create sub-filesystem for templates: %w", err))
		}
		fileSystemForParsing = sub
		trustedFileSystem = template.TrustedFSFromEmbed(*v)
		isEmbed = true
	default:
		panic("templates.New: provided fsys is not an *embed.FS or nil. Due to security constraints in the underlying safehtml/template library, only embedded filesystems or the OS filesystem (when fsys is nil) are supported.")
	}

	t.fileSystem = fileSystemForParsing
	t.fileSystemTrusted = trustedFileSystem
	t.fileSystemIsEmbed = isEmbed
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
		t.AddDynamicBlockCtxToFuncMap()
		t.addTrustedConverterFuncs()
		t.addTrustedConverterFuncsWithContext()
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
	t.templatesLock.Lock()
	defer t.templatesLock.Unlock()
	return t.parseTemplates()
}

// parseTemplates is the internal implementation that expects the caller to hold the lock.
func (t *Templates) parseTemplates() error {
	t.templates = make(map[string]any)
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
			newTemplate, err := t.parseNewTemplateWithFuncMap("", t.funcMap, t.fileSystemTrusted, files...)
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
		newTemplate, err := t.parseNewTemplateWithFuncMap("", t.funcMap, t.fileSystemTrusted, files...)
		if err != nil {
			return fmt.Errorf("%s: %w", pageName, err)
		}
		t.templates[":"+pageName] = newTemplate // sample ':products'
	}
	// Blocks with prefix '_'
	for _, blockFilePath := range blocks {
		blockFilename := filepath.Base(blockFilePath)
		blockName := strings.TrimSuffix(blockFilename, path.Ext(blockFilename))
		newTemplate, err := t.parseNewTemplateWithFuncMap("", t.funcMap, t.fileSystemTrusted, blockFilePath)
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

func definedTemplatesContain(tmpl any, name string) bool {
	switch t := tmpl.(type) {
	case *template.Template:
		templates := t.Templates()
		for _, tmpl := range templates {
			if tmpl.Tree == nil || tmpl.Tree.Root.Pos == 0 {
				continue
			}
			if tmpl.Name() == name {
				return true
			}
		}
	case *text_template.Template:
		templates := t.Templates()
		for _, tmpl := range templates {
			if tmpl.Tree == nil || tmpl.Tree.Root.Pos == 0 {
				continue
			}
			if tmpl.Name() == name {
				return true
			}
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
			err := t.parseTemplates()
			t.templatesLock.Unlock()
			if err != nil {
				return err
			}
		}
	}

	t.templatesLock.RLock()
	defer t.templatesLock.RUnlock()

	// Determine lookupName and execName based on the templateName format
	var lookupName, execName string

	if strings.HasPrefix(templateName, "_") {
		// block/snippet/partial
		// Blocks are stored directly with their name
		lookupName = templateName
		execName = templateName
	} else if strings.HasPrefix(templateName, ":") {
		// page only
		// :page -> lookup "page", execute "page" (the inner define)
		lookupName = templateName
		execName = "page"
	} else if strings.Contains(templateName, ":") {
		// with layout defined in templateName
		// layout:page -> lookup "layout:page", execute "layout"
		lookupName = templateName
		execName = "layout"
	} else {
		// with layout [from request-context or default from config]
		layoutIsSetInContext := false
		if r != nil {
			var layout string
			layout, layoutIsSetInContext = r.Context().Value(LayoutContextKey{}).(string)
			if layoutIsSetInContext {
				lookupName = fmt.Sprint(layout, ":", templateName)
			}
		}
		if !layoutIsSetInContext {
			lookupName = fmt.Sprint(t.DefaultLayout, ":", templateName)
		}
		execName = "layout"
	}

	// check if the template exists
	tmpl, ok := t.templates[lookupName]
	if !ok {
		return fmt.Errorf("%w: %s", ErrTemplateNotFound, lookupName)
	}

	switch tt := tmpl.(type) {
	case *template.Template:
		return tt.ExecuteTemplate(w, execName, data)
	case *text_template.Template:
		return tt.ExecuteTemplate(w, execName, data)
	default:
		return fmt.Errorf("unsupported template type: %T", tmpl)
	}
}

// RenderBlockAsHTMLString renders a specific block to a safehtml.HTML string (or raw string if safehtml is disabled).
// This is useful for rendering partials inside other logic. The block name must
// start with an underscore "_".
func (t *Templates) RenderBlockAsHTMLString(name string, data interface{}) (any, error) {
	if !strings.HasPrefix(name, "_") {
		return nil, fmt.Errorf("%w: blockname needs to start with _", ErrInvalidBlockName)
	}
	if len(name) > 255 {
		return nil, fmt.Errorf("%w: number of characters in string must not exceed 255", ErrInvalidBlockName)
	}
	b := bytes.Buffer{}
	ttRaw, ok := t.templates[name]
	if !ok {
		return nil, fmt.Errorf("%w: template %s not found in templates-map", ErrBlockNotFound, name)
	}

	var err error
	if t.DisableSafeHTML {
		tt := ttRaw.(*text_template.Template)
		err = tt.ExecuteTemplate(&b, name, data)
		return b.String(), err
	}

	tt := ttRaw.(*template.Template)
	err = tt.ExecuteTemplate(&b, name, data)
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(b.String()), err
}

// RenderBlockAsHTMLStringWithContext renders a block to HTML string and logs errors
// with the provided context. Registered as 'd_block_ctx'.
func (t *Templates) RenderBlockAsHTMLStringWithContext(ctx context.Context, blockname string, payload interface{}) (any, error) {
	html, err := t.RenderBlockAsHTMLString(blockname, payload)
	if err != nil {
		t.Logger.ErrorContext(ctx, "d_block_ctx failed", "block", blockname, "error", err)
	}
	return html, err
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

// AddDynamicBlockCtxToFuncMap adds 'd_block_ctx' to the FuncMap.
func (t *Templates) AddDynamicBlockCtxToFuncMap() {
	_, ok := t.funcMap["d_block_ctx"]
	if ok {
		t.Logger.Error("function name is already in use in FuncMap", "name", "d_block_ctx")
		panic("function name 'd_block_ctx' is already in use in FuncMap")
	}
	t.funcMap["d_block_ctx"] = t.RenderBlockAsHTMLStringWithContext
}

// addTrustedConverterFuncs adds the 'trusted_*' functions to the FuncMap.
// These functions wrap strings in the appropriate `safehtml` types (or return them as-is if disabled),
// preventing them from being escaped. Use these only when you are certain the content is
// from a trusted source and is safe to render verbatim.
func (t *Templates) addTrustedConverterFuncs() {
	add := func(name string, f any) {
		if _, ok := t.funcMap[name]; ok {
			t.Logger.Error("function name is already in use in FuncMap", "name", name)
			panic(fmt.Sprintf("function name %q is already in use in FuncMap", name))
		}
		t.funcMap[name] = f
	}

	if t.DisableSafeHTML {
		passthrough := func(v any) any { return v }
		add("trusted_html", passthrough)
		add("trusted_script", passthrough)
		add("trusted_style", passthrough)
		add("trusted_stylesheet", passthrough)
		add("trusted_url", passthrough)
		add("trusted_resource_url", passthrough)
		add("trusted_identifier", passthrough)
	} else {
		add("trusted_html", trustedHTML)
		add("trusted_script", trustedScript)
		add("trusted_style", trustedStyle)
		add("trusted_stylesheet", trustedStyleSheet)
		add("trusted_url", trustedURL)
		add("trusted_resource_url", trustedResourceURL)
		add("trusted_identifier", trustedIdentifier)
	}
}

// addTrustedConverterFuncsWithContext adds 'trusted_*_ctx' functions that log their usage.
func (t *Templates) addTrustedConverterFuncsWithContext() {
	add := func(name string, f any) {
		if _, ok := t.funcMap[name]; ok {
			t.Logger.Error("function name is already in use in FuncMap", "name", name)
			panic(fmt.Sprintf("function name %q is already in use in FuncMap", name))
		}
		t.funcMap[name] = f
	}

	if t.DisableSafeHTML {
		passthroughCtx := func(ctx context.Context, v any) any {
			if !t.DisableTrustedLog {
				s := fmt.Sprint(v)
				t.Logger.InfoContext(ctx, "trusted_helper_ctx called", "content_preview", firstN(s, 50))
			}
			return v
		}
		add("trusted_html_ctx", passthroughCtx)
		add("trusted_script_ctx", passthroughCtx)
		add("trusted_style_ctx", passthroughCtx)
		add("trusted_stylesheet_ctx", passthroughCtx)
		add("trusted_url_ctx", passthroughCtx)
		add("trusted_resource_url_ctx", passthroughCtx)
		add("trusted_identifier_ctx", passthroughCtx)
	} else {
		// Helpers that log the event
		trustedHTMLCtx := func(ctx context.Context, s string) safehtml.HTML {
			if !t.DisableTrustedLog {
				t.Logger.InfoContext(ctx, "trusted_html_ctx called", "content_preview", firstN(s, 50))
			}
			return trustedHTML(s)
		}
		trustedScriptCtx := func(ctx context.Context, s string) safehtml.Script {
			if !t.DisableTrustedLog {
				t.Logger.InfoContext(ctx, "trusted_script_ctx called", "content_preview", firstN(s, 50))
			}
			return trustedScript(s)
		}
		trustedStyleCtx := func(ctx context.Context, s string) safehtml.Style {
			if !t.DisableTrustedLog {
				t.Logger.InfoContext(ctx, "trusted_style_ctx called", "content_preview", firstN(s, 50))
			}
			return trustedStyle(s)
		}
		trustedStyleSheetCtx := func(ctx context.Context, s string) safehtml.StyleSheet {
			if !t.DisableTrustedLog {
				t.Logger.InfoContext(ctx, "trusted_stylesheet_ctx called", "content_preview", firstN(s, 50))
			}
			return trustedStyleSheet(s)
		}
		trustedURLCtx := func(ctx context.Context, s string) safehtml.URL {
			if !t.DisableTrustedLog {
				t.Logger.InfoContext(ctx, "trusted_url_ctx called", "url", s)
			}
			return trustedURL(s)
		}
		trustedResourceURLCtx := func(ctx context.Context, s string) safehtml.TrustedResourceURL {
			if !t.DisableTrustedLog {
				t.Logger.InfoContext(ctx, "trusted_resource_url_ctx called", "url", s)
			}
			return trustedResourceURL(s)
		}
		trustedIdentifierCtx := func(ctx context.Context, s string) safehtml.Identifier {
			if !t.DisableTrustedLog {
				t.Logger.InfoContext(ctx, "trusted_identifier_ctx called", "identifier", s)
			}
			return trustedIdentifier(s)
		}

		add("trusted_html_ctx", trustedHTMLCtx)
		add("trusted_script_ctx", trustedScriptCtx)
		add("trusted_style_ctx", trustedStyleCtx)
		add("trusted_stylesheet_ctx", trustedStyleSheetCtx)
		add("trusted_url_ctx", trustedURLCtx)
		add("trusted_resource_url_ctx", trustedResourceURLCtx)
		add("trusted_identifier_ctx", trustedIdentifierCtx)
	}
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
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

func (t *Templates) parseNewTemplateWithFuncMap(layout string, fnMap template.FuncMap, fsys template.TrustedFS, files ...string) (any, error) {
	if len(files) == 0 {
		return nil, errors.New("no files in slice")
	}

	if t.DisableSafeHTML {
		fm := make(text_template.FuncMap, len(fnMap))
		for k, v := range fnMap {
			fm[k] = v
		}
		tmpl := text_template.New(layout).Funcs(fm)
		return tmpl.ParseFS(t.fileSystem, files...)
	}

	tmpl := template.New(layout).Funcs(fnMap)
	return tmpl.ParseFS(fsys, files...)
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

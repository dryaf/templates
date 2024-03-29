# templates

[![License](http://img.shields.io/badge/license-mit-blue.svg?style=flat-square)](https://raw.githubusercontent.com/dryaf/templates/master/LICENSE)
[![Coverage](https://raw.githubusercontent.com/dryaf/templates/master/coverage.svg)](https://raw.githubusercontent.com/dryaf/templates/master/coverage.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/dryaf/templates?style=flat-square)](https://goreportcard.com/report/github.com/dryaf/templates)
[![GoDoc](https://godoc.org/github.com/dryaf/templates?status.svg)](https://godoc.org/github.com/dryaf/templates)

## What is it about?

1. allows you to have layouts, pages, blocks (partials) 
2. allows you to live reload public files and templates in dev mode and use embeded files in production (golang's embed)
3. allows you to render dynamically set blocks via 'd-block' instead of 'block'
4. d-block allows you to use a headless-cms that serves json and assets only

## Usage with echo (not needed):

### 1. Create directories and files:

- files
    - public
        - override.css
        - hopefullynot.js
    - templates
        - layouts
            - application.gohtml
            - special.gohtml  
        - pages **(must contain define 'page' - all of em, always 'page' - not 'todos_show' etc.)**
            - todos_show.gohtml
            - todos_index.gohtml
            - todos_edit.gohtml
            - ...
        - blocks **(definition within file must start with _ and must contain definition '_form')**
            - form.gohtml
            - message.gohtml


### 2. Initialize:
```go

    import echo_integration "github.com/templates/integrations/echo"

    ...


	// (embeded) public files and templates
	fileSystem := getFileSystem("files", in_dev_mode)
	templates := templates.New(fileSystem, "./templates", nil)
	templates.AlwaysReloadAndParseTemplates = in_dev_mode
	templates.MustParseTemplates()

	// sample echo
	e := echo.New()
	e.Pre(echo_integration.MethodOverrideFormField("_method"))
	e.Use(echo_integration.CSRFTokenLookup("form:csrf"))
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Renderer = echo_integration.Renderer(templates)
```

### 3. Use in handler
```go
func (ht *Handlers) Todos(c echo.Context) error {
	...
	return c.Render(200, "todos_index", data) // renders page with default layout 'application'
    // or
    return c.Render(200, "special:todos_index", data) // renders the page with the 'special' layout
    // or
    return c.Render(200, ":todos_index", data) // renders the page without a layout 
    // or
    return c.Render(200, "_message", data) // renders only the block/partial/snippet/... without a page or layout

    // without echo
   templates.ExecuteTemplate(respone, request, "_message or as above", data)
}

```

# Locals can be passed to blocks

```html
{{block "_sample_block_with_locals" locals "a" 1 "b" 2 "c" 3  }}{{end}}
{{locals "a" "x" "b" "y" "c" "z" | d_block "_sample_block_with_locals"   }}
```

## Todos till v0.1.0
[x] use safehtml instead of std lib
[x] move integrations_echo to seperate package
[ ] code cleanup
[ ] implement sse for hotwired turbo
[ ] add tests for hotwired turbo

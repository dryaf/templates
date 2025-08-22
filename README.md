# templates

[![License](http://img.shields.io/badge/license-mit-blue.svg?style=flat-square)](https://raw.githubusercontent.com/dryaf/templates/master/LICENSE)
[![Coverage](https://raw.githubusercontent.com/dryaf/templates/master/coverage.svg)](https://raw.githubusercontent.com/dryaf/templates/master/coverage.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/dryaf/templates?style=flat-square)](https://goreportcard.com/report/github.com/dryaf/templates)
[![GoDoc](https://godoc.org/github.com/dryaf/templates?status.svg)](https://godoc.org/github.com/dryaf/templates)

A secure, file-system-based Go template engine built on Google's `safehtml/template`. It provides a familiar structure of layouts, pages, and reusable blocks (partials) while ensuring output is safe from XSS vulnerabilities by default.

## Why this library?

Go's standard `html/template` is good, but Google's `safehtml/template` is better, providing superior, context-aware automatic escaping that offers stronger security guarantees against XSS. However, `safehtml/template` can be complex to set up, especially for projects using a traditional layout/page/partial structure.

This library provides a simple, opinionated framework around `safehtml/template` so you can get the security benefits without the setup overhead.

## Features

- **Secure by Default**: Built on `safehtml/template` to provide contextual, automatic output escaping.
- **Layouts, Pages, and Blocks**: Organizes templates into a familiar and powerful structure. Render pages within different layouts, or render blocks individually.
- **Live Reloading**: Automatically re-parses templates on every request for a seamless development experience.
- **Production-Ready**: Uses Go's `embed.FS` to compile all templates and assets into a single binary for production deployments.
- **Dynamic Rendering**: Includes a `d_block` helper to render blocks dynamically by name—perfect for headless CMS integrations.
- **Convenient Helpers**: Comes with a `locals` function to easily pass key-value data to blocks.
- **Framework Integrations**: Provides optional, lightweight integration packages for `net/http`, `Echo`, `chi`, and `gin-gonic/gin`.

## Installation

Add the library to your `go.mod` file:
```shell
go get github.com/dryaf/templates
```

Then import it in your code:
```go
import "github.com/dryaf/templates"
```

## Quick Start

1.  **Create your template files:**

    ```
    .
    └── files
        └── templates
            ├── layouts
            │   └── application.gohtml
            └── pages
                └── home.gohtml
    ```

    `files/templates/layouts/application.gohtml`:
    ```html
    {{define "layout"}}
    <!DOCTYPE html>
    <html><body>
        <h1>Layout</h1>
        {{block "page" .}}{{end}}
    </body></html>
    {{end}}
    ```

    `files/templates/pages/home.gohtml`:
    ```html
    {{define "page"}}
    <h2>Home Page</h2>
    <p>Hello, {{.}}!</p>
    {{end}}
    ```

2.  **Write your Go application:**

    ```go
    package main

    import (
    	"log"
    	"net/http"

    	"github.com/dryaf/templates"
    	"github.com/dryaf/templates/integrations/stdlib"
    )

    func main() {
    	// For development, New(nil, nil) uses the local file system.
    	// For production, you would pass in an embed.FS.
    	tmpls := templates.New(nil, nil)
    	tmpls.AlwaysReloadAndParseTemplates = true // Recommended for development
    	tmpls.MustParseTemplates()

    	renderer := stdlib.FromTemplates(tmpls)

    	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    		err := renderer.Render(w, r, http.StatusOK, "home", "World")
    		if err != nil {
    			log.Println(err)
    			http.Error(w, "Internal Server Error", 500)
    		}
    	})

    	log.Println("Starting server on :8080")
    	http.ListenAndServe(":8080", nil)
    }
    ```

## Running the Examples

The project includes a comprehensive set of runnable examples in the `_examples` directory. To run them:

1.  **Set up the template files:** The examples use a shared set of templates. A `Makefile` target is provided to copy them into place. From the project root, run:
    ```shell
    make setup-examples
    ```

2.  **Run an example:** Navigate to any example directory and run it.
    ```shell
    cd _examples/chi
    go run .
    ```

3.  **Clean up:** To remove the copied template files, run:
    ```shell
    make clean-examples
    ```

## Core Concepts

### Directory Structure

The engine expects a specific directory structure by default, located at `./files/templates`:

-   `files/templates/layouts/`: Contains layout templates. Each file defines a "layout".
-   `files/templates/pages/`: Contains page templates.
-   `files/templates/blocks/`: Contains reusable blocks (partials).

### Important Template Rules

1.  **Pages must define `"page"`**: Every template file in the `pages` directory must define its main content within `{{define "page"}}...{{end}}`.
2.  **Blocks must define `"_name"`**: Every template file in the `blocks` directory must define a template, and that definition's name must match the filename and be prefixed with an underscore. For example, `_form.gohtml` must contain `{{define "_form"}}...{{end}}`.

### Rendering Syntax

You have fine-grained control over how templates are rendered:

-   `"page_name"`: Renders the page within the default layout (`application.gohtml`).
-   `"layout_name:page_name"`: Renders the page within a specific layout.
-   `":page_name"`: Renders the page without any layout.
-   `"_block_name"`: Renders a specific block by itself.

### Dynamic Blocks (`d_block`)

To render a block whose name is determined at runtime (e.g., from a database or CMS API), you can use `d_block`. This is a powerful feature for dynamic page composition.

```html
<!-- Instead of this, which requires the block name to be static: -->
{{block "_header" .}}{{end}}

<!-- You can do this: -->
{{d_block .HeaderBlockName .HeaderBlockData}}
```

### Passing `locals` to Blocks

Passing maps as context to blocks can be verbose. The `locals` helper function makes it easy to create a map on the fly. It accepts a sequence of key-value pairs.

```html
<!-- Standard block call with locals -->
{{block "_user_card" (locals "Name" "Alice" "Age" 30)}}{{end}}

<!-- Dynamic block call with locals -->
{{locals "Name" "Bob" "Age" 42 | d_block "_user_card"}}
```

`_user_card.gohtml`:
```html
{{define "_user_card"}}
<div class="card">
    <h3>{{.Name}}</h3>
    <p>Age: {{.Age}}</p>
</div>
{{end}}
```

### Security and `trusted_*` Functions

This library uses `safehtml/template`, which provides protection against XSS by default. It contextually escapes variables.

Sometimes, you receive data from a trusted source (like a headless CMS) that you know is safe and should not be escaped. For these cases, you can use the `trusted_*` template functions, which wrap the input string in the appropriate `safehtml` type.

-   `trusted_html`: For HTML content.
-   `trusted_script`: For JavaScript code.
-   `trusted_style`: For CSS style declarations.
-   `trusted_stylesheet`: For a full CSS stylesheet.
-   `trusted_url`: For a general URL.
-   `trusted_resource_url`: For a URL that loads a resource like a script or stylesheet.
-   `trusted_identifier`: For an HTML ID or name attribute.

**Example:**
```html
<!-- This will be escaped: -->
<p>{{.UnsafeHTMLFromUser}}</p>

<!-- This will be rendered verbatim, because you are vouching for its safety: -->
<div>
    {{trusted_html .SafeHTMLFromCMS}}
</div>
```

## Integrations

### `net/http` (stdlib)

The `integrations/stdlib` package provides a simple renderer for use with `net/http`.

```go
import "github.com/dryaf/templates/integrations/stdlib"

// --- inside main ---
tmpls := templates.New(nil, nil) // or with embed.FS for production
tmpls.MustParseTemplates()
renderer := stdlib.FromTemplates(tmpls)

// Use it in an http.HandlerFunc
http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    renderer.Render(w, r, http.StatusOK, "home", "Data")
})

// Or create a handler that always renders the same template
http.Handle("/about", renderer.Handler("about", nil))
```

### Echo

The `integrations/echo` package provides a renderer for the [Echo framework](https://echo.labstack.com/).

```go
import "github.com/dryaf/templates/integrations/echo"
// ...
e := echo.New()
e.Renderer = templates_echo.Renderer(tmpls)

e.GET("/", func(c echo.Context) error {
    return c.Render(http.StatusOK, "home", "World")
})
```

### Chi

The `integrations/chi` package provides a renderer compatible with the `chi` router.

```go
import "github.com/dryaf/templates/integrations/chi"
// ...
renderer := chi.FromTemplates(tmpls)
r := chi.NewRouter()

r.Get("/", func(w http.ResponseWriter, r *http.Request) {
    renderer.Render(w, r, http.StatusOK, "home", "Chi")
})
```

### Gin

The `integrations/gin` package provides a renderer that implements `gin.HTMLRender` for the [Gin framework](https://gin-gonic.com/).

```go
import "github.com/dryaf/templates/integrations/gin"
// ...
router := gin.Default()
router.HTMLRender = templates_gin.New(tmpls)

router.GET("/", func(c *gin.Context) {
    c.HTML(http.StatusOK, "home", "Gin")
})
```

## Roadmap

The library is considered feature-complete and stable for a v1.0.0 release. Future development will be driven by community feedback and integration requests for new frameworks. Potential ideas include:

-   Additional template helper functions.
-   Performance optimizations.

Feel free to open an issue to suggest features or improvements.

## License

This project is licensed under the MIT License.

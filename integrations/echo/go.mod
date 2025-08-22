module github.com/dryaf/templates/integrations/echo

go 1.24.0

toolchain go1.24.5

require (
	github.com/dryaf/templates v0.0.0
	github.com/google/safehtml v0.1.1-0.20250618200626-e177c9cd28ca
	github.com/labstack/echo/v4 v4.13.4
)

require (
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/time v0.12.0 // indirect
)

replace github.com/dryaf/templates => ../..

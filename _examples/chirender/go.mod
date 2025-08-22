// ==== File: _examples/chirender/go.mod ====
module github.com/dryaf/templates/_examples/chirender

go 1.24

require (
	github.com/dryaf/templates v1.0.0
	github.com/dryaf/templates/integrations/chirender v0.0.0-20250822221651-2b72f52fe807
	github.com/go-chi/chi/v5 v5.2.2
	github.com/go-chi/render v1.0.3
)

require (
	github.com/ajg/form v1.5.1 // indirect
	github.com/google/safehtml v0.1.1-0.20250618200626-e177c9cd28ca // indirect
	golang.org/x/text v0.28.0 // indirect
)

replace github.com/dryaf/templates => ../..

// ==== File: integrations/stdlib/go.mod ====
module github.com/dryaf/templates/integrations/stdlib

go 1.24

require (
	github.com/dryaf/templates v1.0.4
	github.com/google/safehtml v0.1.1-0.20250618200626-e177c9cd28ca
)

require golang.org/x/text v0.28.0 // indirect

replace github.com/dryaf/templates => ../..

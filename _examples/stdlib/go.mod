module github.com/dryaf/templates/_examples/stdlib

go 1.24

require (
	github.com/dryaf/templates v0.1.2
	github.com/dryaf/templates/integrations/stdlib v0.0.0-20250822221651-2b72f52fe807
)

require (
	github.com/google/safehtml v0.1.1-0.20250618200626-e177c9cd28ca // indirect
	golang.org/x/text v0.28.0 // indirect
)

replace github.com/dryaf/templates => ../..

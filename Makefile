// ==== File: Makefile ====
.PHONY: test coverage-svg template-contents setup-examples clean-examples deps-update

test:
	@echo "Running tests for main module..."
	go test ./... -coverpkg=./... -coverprofile=coverage-main.out
	@echo "Running tests for stdlib integration module..."
	(cd integrations/stdlib && go test -coverprofile=../../coverage-stdlib.out)
	@echo "Running tests for echo integration module..."
	(cd integrations/echo && go test -coverprofile=../../coverage-echo.out)
	@echo "Running tests for chi integration module..."
	(cd integrations/chi && go test -coverprofile=../../coverage-chi.out)
	@echo "Running tests for chi-render integration module..."
	(cd integrations/chirender && go test -coverprofile=../../coverage-chirender.out)
	@echo "Running tests for gin integration module..."
	(cd integrations/gin && go test -coverprofile=../../coverage-gin.out)
	@echo "Combining coverage reports..."
	@echo "mode: set" > coverage.out
	@tail -n +2 coverage-main.out >> coverage.out
	@tail -n +2 coverage-stdlib.out >> coverage.out
	@tail -n +2 coverage-echo.out >> coverage.out
	@tail -n +2 coverage-chi.out >> coverage.out
	@tail -n +2 coverage-chirender.out >> coverage.out
	@tail -n +2 coverage-gin.out >> coverage.out
	@rm coverage-main.out coverage-stdlib.out coverage-echo.out coverage-chi.out coverage-chirender.out coverage-gin.out

template-contents:
	find ./files/templates -type f -exec echo "==> {} <==" \; -exec cat {} \; -exec echo \;

coverage-svg: test
	@output=$$(go tool cover -func ./coverage.out 2>/dev/null) && \
	percentage=$$(echo "$$output" | grep 'total:' | awk '{print $$3}') && \
	svg_content="<svg xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\" width=\"108\" height=\"20\" role=\"img\" aria-label=\"coverage: $${percentage}\"><title>coverage: $${percentage}</title><linearGradient id=\"s\" x2=\"0\" y2=\"100%\"><stop offset=\"0\" stop-color=\"#bbb\" stop-opacity=\".1\"/><stop offset=\"1\" stop-opacity=\".1\"/></linearGradient><clipPath id=\"r\"><rect width=\"108\" height=\"20\" rx=\"3\" fill=\"#fff\"/></clipPath><g clip-path=\"url(#r)\"><rect width=\"61\" height=\"20\" fill=\"#555\"/><rect x=\"61\" width=\"47\" height=\"20\" fill=\"#97ca00\"/><rect width=\"108\" height=\"20\" fill=\"url(#s)\"/></g><g fill=\"#fff\" text-anchor=\"middle\" font-family=\"Verdana,Geneva,DejaVu Sans,sans-serif\" text-rendering=\"geometricPrecision\" font-size=\"110\"><text aria-hidden=\"true\" x=\"315\" y=\"150\" fill=\"#010101\" fill-opacity=\".3\" transform=\"scale(.1)\" textLength=\"510\">coverage</text><text x=\"315\" y=\"140\" transform=\"scale(.1)\" fill=\"#fff\" textLength=\"510\">coverage</text><text aria-hidden=\"true\" x=\"835\" y=\"150\" fill=\"#010101\" fill-opacity=\".3\" transform=\"scale(.1)\" textLength=\"370\">$${percentage}</text><text x=\"835\" y=\"140\" transform=\"scale(.1)\" fill=\"#fff\" textLength=\"370\">$${percentage}</text></g></svg>" && \
	echo "$$svg_content" > coverage.svg

# Define the example directories
EXAMPLE_DIRS := _examples/stdlib _examples/chi _examples/chirender _examples/echo _examples/gin

# Target to set up copied files for all examples
setup-examples:
	@echo "Setting up copied 'files' directories for examples..."
	@for dir in $(EXAMPLE_DIRS); do \
		if [ ! -e $$dir/files ]; then \
			echo "Copying _examples/files to $$dir/files..."; \
			cp -R _examples/files $$dir/files; \
		else \
			echo "'files' directory already exists in $$dir, skipping."; \
		fi; \
	done
	@echo "Setup complete."

# Target to clean up copied files from all examples
clean-examples:
	@echo "Cleaning up copied 'files' directories for examples..."
	@for dir in $(EXAMPLE_DIRS); do \
		target_dir=$$dir/files; \
		if [ -d "$$target_dir" ]; then \
			echo "Removing directory: $$target_dir"; \
			rm -rf "$$target_dir"; \
		else \
			echo "No 'files' directory to remove in $$dir."; \
		fi; \
	done
	@echo "Cleanup complete."

# Target to update all Go module dependencies in the project
deps-update:
	@echo "Updating Go module dependencies for all modules..."
	@# Find all go.mod files, then for each one, cd into its directory and update dependencies.
	@find . -name "go.mod" -print0 | xargs -0 -I {} sh -c ' \
		dir=$$(dirname {}); \
		echo "--> Updating dependencies in $$dir"; \
		(cd $$dir && go get -u ./... && go mod tidy); \
	'
	@echo "Dependency update complete."
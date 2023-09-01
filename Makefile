.PHONY: coverage

template-contents:
	find ./files/templates -type f -exec echo "==> {} <==" \; -exec cat {} \; -exec echo \;

coverage-svg:
	@output=$$(go test ./...  -coverpkg=./... -coverprofile ./coverage.out 2>/dev/null && go tool cover -func ./coverage.out 2>/dev/null) && \
	percentage=$$(echo "$$output" | grep 'total:' | awk '{print $$3}') && \
	svg_content="<svg xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\" width=\"108\" height=\"20\" role=\"img\" aria-label=\"coverage: $${percentage}\"><title>coverage: $${percentage}</title><linearGradient id=\"s\" x2=\"0\" y2=\"100%\"><stop offset=\"0\" stop-color=\"#bbb\" stop-opacity=\".1\"/><stop offset=\"1\" stop-opacity=\".1\"/></linearGradient><clipPath id=\"r\"><rect width=\"108\" height=\"20\" rx=\"3\" fill=\"#fff\"/></clipPath><g clip-path=\"url(#r)\"><rect width=\"61\" height=\"20\" fill=\"#555\"/><rect x=\"61\" width=\"47\" height=\"20\" fill=\"#97ca00\"/><rect width=\"108\" height=\"20\" fill=\"url(#s)\"/></g><g fill=\"#fff\" text-anchor=\"middle\" font-family=\"Verdana,Geneva,DejaVu Sans,sans-serif\" text-rendering=\"geometricPrecision\" font-size=\"110\"><text aria-hidden=\"true\" x=\"315\" y=\"150\" fill=\"#010101\" fill-opacity=\".3\" transform=\"scale(.1)\" textLength=\"510\">coverage</text><text x=\"315\" y=\"140\" transform=\"scale(.1)\" fill=\"#fff\" textLength=\"510\">coverage</text><text aria-hidden=\"true\" x=\"835\" y=\"150\" fill=\"#010101\" fill-opacity=\".3\" transform=\"scale(.1)\" textLength=\"370\">$${percentage}</text><text x=\"835\" y=\"140\" transform=\"scale(.1)\" fill=\"#fff\" textLength=\"370\">$${percentage}</text></g></svg>" && \
	echo "$$svg_content" > coverage.svg

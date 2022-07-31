package templates

import (
	"context"
	"net/http"
	"strings"
)

func isValidHttpMethod(m string) bool {
	switch strings.ToUpper(m) {
	case http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodConnect,
		http.MethodTrace:
		return true
	default:
		return false
	}
}

// LayoutContextKey is the key for getting the layout string out of the context
type LayoutContextKey struct{}

// OtherLayoutIfXMLHttpRequest seeks for unpoly header and sets LayoutContextKey to partial if it exists
func OtherLayoutIfXMLHttpRequest(next http.Handler, otherLayout string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestType := r.Header.Get("X-Requested-With")
		layout := "application"
		if requestType == "XMLHttpRequest" {
			layout = otherLayout
		}
		ctx := context.WithValue(r.Context(), LayoutContextKey{}, layout)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

package templates

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/safehtml/template"
	"golang.org/x/exp/slog"
)

func fatalOnErr(err error) {
	if err != nil {
		slog.Error("fatal", err)
		os.Exit(1)
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
	files := []string{}
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

func parseNewTemplateWithFuncMap(layout string, fnMap template.FuncMap, fs template.TrustedFS, files ...string) (*template.Template, error) {
	if len(files) == 0 {
		return nil, errors.New("no files in slice")
	}
	t := template.New(layout).Funcs(fnMap)

	t, err := t.ParseFS(fs, files...)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// cleanPath returns the canonical path for p, eliminating . and .. elements.
// taken from https://golang.org/src/net/http/server.go?s=68684:68715#L2203
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	// if p[0] != '/' {
	// 	p = "/" + p
	// }
	np := path.Clean(p)
	// path.Clean removes trailing slash except for root;
	// put the trailing slash back if necessary.
	if p[len(p)-1] == '/' && np != "/" {
		// Fast path for common case of p being the string we want:
		if len(p) == len(np)+1 && strings.HasPrefix(p, np) {
			np = p
		} else {
			np += "/"
		}
	}
	return np
}

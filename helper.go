package templates

import (
	"html/template"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/exp/slog"
)

func fatalOnErr(err error) {
	if err != nil {
		slog.Error("fatal", err)
		os.Exit(1)
	}
}

func getFilePathsInDir(fs http.FileSystem, dirPath string, fileExtension string, blocksOnly bool) ([]string, error) {
	dir, err := fs.Open(cleanPath(dirPath))
	if err != nil {
		return nil, errors.Wrap(err, "opening layout dir")
	}
	fileInfos, err := dir.Readdir(-1)
	if err != nil {
		return nil, errors.Wrap(err, "reading layout dir")
	}
	files := []string{}
	for _, fileInfo := range fileInfos {
		if blocksOnly && path.Base(fileInfo.Name())[:1] != "_" {
			continue
		}
		if path.Ext(fileInfo.Name()) == fileExtension {
			files = append(files, cleanPath(filepath.Join(dirPath, fileInfo.Name())))
		}
	}
	return files, nil
}

func parseNewTemplateWithFuncMap(layout string, fnMap template.FuncMap, fs http.FileSystem, filenames ...string) (*template.Template, error) {
	if len(filenames) == 0 {
		return nil, errors.New("no files in slice")
	}
	t := template.New(layout).Funcs(fnMap)
	for _, filename := range filenames {
		file, err := fs.Open(filename)
		if err != nil {
			return nil, errors.Wrap(err, filename)
		}
		b, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}

		t, err = t.Parse(string(b))

		if err != nil {
			return nil, err
		}
	}
	return t, nil
}

// cleanPath returns the canonical path for p, eliminating . and .. elements.
// taken from https://golang.org/src/net/http/server.go?s=68684:68715#L2203
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
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

package templates

import "path/filepath"

type TemplatesConfig struct {
	DefaultLayout                string
	TemplateFileExtension        string
	LayoutsPath                  string
	PagesPath                    string
	BlocksPath                   string
	AddHeadlessCMSFuncMapHelpers bool
}

func DefaultTemplatesConfig(templatesPath string) *TemplatesConfig {
	return &TemplatesConfig{
		DefaultLayout:         "application",
		TemplateFileExtension: ".gohtml",
		LayoutsPath:           filepath.Join(templatesPath, "layouts"),
		PagesPath:             filepath.Join(templatesPath, "pages"),
		BlocksPath:            filepath.Join(templatesPath, "blocks"),

		AddHeadlessCMSFuncMapHelpers: true, // d_block, trust_html
	}
}

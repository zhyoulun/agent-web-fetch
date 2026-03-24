package sites

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/wikipedia.playwright.js.tmpl
var wikipediaPlaywrightScriptTemplate string

var wikipediaPlaywrightScriptParser = template.Must(template.New("wikipedia-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(wikipediaPlaywrightScriptTemplate))

func RenderWikipediaPlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := wikipediaPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 wikipedia 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

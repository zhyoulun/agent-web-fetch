package search

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

//go:embed scripts/imdb.playwright.js.tmpl
var imdbPlaywrightScriptTemplate string

var imdbPlaywrightScriptParser = template.Must(template.New("imdb-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(imdbPlaywrightScriptTemplate))

func RenderIMDbPlaywrightScript(data sites.PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := imdbPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 imdb 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

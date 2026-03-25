package search

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

//go:embed scripts/piratebay.playwright.js.tmpl
var pirateBayPlaywrightScriptTemplate string

var pirateBayPlaywrightScriptParser = template.Must(template.New("piratebay-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(pirateBayPlaywrightScriptTemplate))

func RenderPirateBayPlaywrightScript(data sites.PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := pirateBayPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 piratebay 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

package search

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

//go:embed scripts/douban.playwright.js.tmpl
var douBanPlaywrightScriptTemplate string

var douBanPlaywrightScriptParser = template.Must(template.New("douban-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(douBanPlaywrightScriptTemplate))

func RenderDouBanPlaywrightScript(data sites.PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := douBanPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 douban 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

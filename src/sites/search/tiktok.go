package search

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/tiktok.playwright.js.tmpl
var tiktokPlaywrightScriptTemplate string

var tiktokPlaywrightScriptParser = template.Must(template.New("tiktok-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(tiktokPlaywrightScriptTemplate))

func RenderTikTokPlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := tiktokPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 tiktok 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

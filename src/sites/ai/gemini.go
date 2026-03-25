package ai

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

//go:embed scripts/gemini.playwright.js.tmpl
var geminiPlaywrightScriptTemplate string

var geminiPlaywrightScriptParser = template.Must(template.New("gemini-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(geminiPlaywrightScriptTemplate))

func RenderGeminiPlaywrightScript(data sites.PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := geminiPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 gemini 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

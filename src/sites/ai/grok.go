package ai

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

//go:embed scripts/grok.playwright.js.tmpl
var grokPlaywrightScriptTemplate string

var grokPlaywrightScriptParser = template.Must(template.New("grok-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(grokPlaywrightScriptTemplate))

func RenderGrokPlaywrightScript(data sites.PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := grokPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 grok 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

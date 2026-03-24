package sites

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/google.playwright.js.tmpl
var googlePlaywrightScriptTemplate string

var googlePlaywrightScriptParser = template.Must(template.New("google-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(googlePlaywrightScriptTemplate))

func RenderGooglePlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := googlePlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 google 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

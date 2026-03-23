package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/duckduckgo.playwright.js.tmpl
var duckDuckGoPlaywrightScriptTemplate string

var duckDuckGoPlaywrightScriptParser = template.Must(template.New("duckduckgo-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(duckDuckGoPlaywrightScriptTemplate))

func renderDuckDuckGoPlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := duckDuckGoPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 duckduckgo 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

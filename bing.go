package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/bing.playwright.js.tmpl
var bingPlaywrightScriptTemplate string

var bingPlaywrightScriptParser = template.Must(template.New("bing-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(bingPlaywrightScriptTemplate))

func renderBingPlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := bingPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 bing 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

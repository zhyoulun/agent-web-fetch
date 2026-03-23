package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/temu.playwright.js.tmpl
var temuPlaywrightScriptTemplate string

var temuPlaywrightScriptParser = template.Must(template.New("temu-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(temuPlaywrightScriptTemplate))

func renderTemuPlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := temuPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 temu 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

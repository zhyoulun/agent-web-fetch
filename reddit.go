package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/reddit.playwright.js.tmpl
var redditPlaywrightScriptTemplate string

var redditPlaywrightScriptParser = template.Must(template.New("reddit-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(redditPlaywrightScriptTemplate))

func renderRedditPlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := redditPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 reddit 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

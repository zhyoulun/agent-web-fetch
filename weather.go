package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/weather.playwright.js.tmpl
var weatherPlaywrightScriptTemplate string

var weatherPlaywrightScriptParser = template.Must(template.New("weather-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(weatherPlaywrightScriptTemplate))

func renderWeatherPlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := weatherPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 weather 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

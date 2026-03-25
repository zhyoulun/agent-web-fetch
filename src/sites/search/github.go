package search

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

//go:embed scripts/github.playwright.js.tmpl
var githubPlaywrightScriptTemplate string

var githubPlaywrightScriptParser = template.Must(template.New("github-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(githubPlaywrightScriptTemplate))

func RenderGitHubPlaywrightScript(data sites.PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := githubPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 github 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

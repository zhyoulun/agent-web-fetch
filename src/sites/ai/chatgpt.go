package ai

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

//go:embed scripts/chatgpt.playwright.js.tmpl
var chatGPTPlaywrightScriptTemplate string

var chatGPTPlaywrightScriptParser = template.Must(template.New("chatgpt-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(chatGPTPlaywrightScriptTemplate))

func RenderChatGPTPlaywrightScript(data sites.PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := chatGPTPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 chatgpt 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

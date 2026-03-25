package search

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

//go:embed scripts/youtube.playwright.js.tmpl
var youtubePlaywrightScriptTemplate string

var youtubePlaywrightScriptParser = template.Must(template.New("youtube-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(youtubePlaywrightScriptTemplate))

func RenderYouTubePlaywrightScript(data sites.PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := youtubePlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 youtube 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

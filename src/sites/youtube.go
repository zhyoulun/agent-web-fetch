package sites

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/youtube.playwright.js.tmpl
var youtubePlaywrightScriptTemplate string

var youtubePlaywrightScriptParser = template.Must(template.New("youtube-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(youtubePlaywrightScriptTemplate))

func RenderYouTubePlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := youtubePlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 youtube 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

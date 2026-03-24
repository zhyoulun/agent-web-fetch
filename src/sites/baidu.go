package sites

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"text/template"
)

//go:embed scripts/baidu.playwright.js.tmpl
var baiduPlaywrightScriptTemplate string

var baiduPlaywrightScriptParser = template.Must(template.New("baidu-playwright-script").Funcs(template.FuncMap{
	"js": func(input string) string {
		return strconv.Quote(input)
	},
}).Parse(baiduPlaywrightScriptTemplate))

func RenderBaiduPlaywrightScript(data PlaywrightScriptData) (string, error) {
	var buf bytes.Buffer
	if err := baiduPlaywrightScriptParser.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 baidu 脚本模板失败: %w", err)
	}
	return buf.String(), nil
}

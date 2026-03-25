package src

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
	"github.com/zhyoulun/agent-web-fetch/src/sites/ai"
	"github.com/zhyoulun/agent-web-fetch/src/sites/search"
)

type Renderer func(data sites.PlaywrightScriptData) (string, error)

type PlaywrightFactory struct {
	renderers map[string]Renderer
}

func NewPlaywrightFactory() *PlaywrightFactory {
	return &PlaywrightFactory{
		renderers: map[string]Renderer{
			"amazon":     search.RenderAmazonPlaywrightScript,
			"baidu":      search.RenderBaiduPlaywrightScript,
			"bing":       search.RenderBingPlaywrightScript,
			"chatgpt":    ai.RenderChatGPTPlaywrightScript,
			"duckduckgo": search.RenderDuckDuckGoPlaywrightScript,
			"gemini":     ai.RenderGeminiPlaywrightScript,
			"google":     search.RenderGooglePlaywrightScript,
			"grok":       ai.RenderGrokPlaywrightScript,
			"reddit":     search.RenderRedditPlaywrightScript,
			"tiktok":     search.RenderTikTokPlaywrightScript,
			"wikipedia":  search.RenderWikipediaPlaywrightScript,
			"youtube":    search.RenderYouTubePlaywrightScript,
		},
	}
}

func (f *PlaywrightFactory) Render(data sites.PlaywrightScriptData) (string, error) {
	engine := strings.ToLower(strings.TrimSpace(data.Engine))
	renderer, ok := f.renderers[engine]
	if !ok {
		return "", fmt.Errorf("不支持的搜索引擎: %s", engine)
	}
	return renderer(data)
}

func (f *PlaywrightFactory) Supports(engine string) bool {
	_, ok := f.renderers[strings.ToLower(strings.TrimSpace(engine))]
	return ok
}

func (f *PlaywrightFactory) Engines() []string {
	engines := make([]string, 0, len(f.renderers))
	for engine := range f.renderers {
		engines = append(engines, engine)
	}
	sort.Strings(engines)
	return engines
}

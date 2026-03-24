package src

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

type Renderer func(data sites.PlaywrightScriptData) (string, error)

type PlaywrightFactory struct {
	renderers map[string]Renderer
}

func NewPlaywrightFactory() *PlaywrightFactory {
	return &PlaywrightFactory{
		renderers: map[string]Renderer{
			"amazon":     sites.RenderAmazonPlaywrightScript,
			"baidu":      sites.RenderBaiduPlaywrightScript,
			"bing":       sites.RenderBingPlaywrightScript,
			"duckduckgo": sites.RenderDuckDuckGoPlaywrightScript,
			"google":     sites.RenderGooglePlaywrightScript,
			"reddit":     sites.RenderRedditPlaywrightScript,
			"tiktok":     sites.RenderTikTokPlaywrightScript,
			"wikipedia":  sites.RenderWikipediaPlaywrightScript,
			"youtube":    sites.RenderYouTubePlaywrightScript,
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

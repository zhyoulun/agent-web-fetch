package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type SearchResult struct {
	Title string
	URL   string
}

type ScriptStatus string

const (
	ScriptStatusOK                ScriptStatus = "ok"
	ScriptStatusHumanVerification ScriptStatus = "human_verification"
	ScriptStatusError             ScriptStatus = "error"
)

type ScriptResult struct {
	Status       ScriptStatus   `json:"status"`
	Reason       string         `json:"reason"`
	Error        string         `json:"error"`
	Results      []SearchResult `json:"results"`
	SnapshotPath string         `json:"snapshotPath"`
}

type PlaywrightScriptData struct {
	// Engine 是搜索引擎名称
	Engine string
	// Query 是搜索关键词
	Query string
	// ProfileDir 是浏览器用户目录，用于复用登录态与 cookie
	ProfileDir string
	// Channel 是 Playwright 浏览器通道，如 chrome/chromium/msedge
	Channel string
	// MaxResults 是最多返回结果数
	MaxResults int
	// TimeoutMS 是页面等待与操作超时时间（毫秒）
	TimeoutMS int64
	// HeadlessMode 支持 true/false/first
	HeadlessMode string
	// Snapshot 控制是否输出搜索结果页截图
	Snapshot bool
	// SnapshotStamp 用于截图目录时间戳
	SnapshotStamp string
	// ProjectRoot 是项目根目录，用于拼接输出路径
	ProjectRoot string
	// OutputPath 是脚本回写 JSON 结果文件路径
	OutputPath string
}

func main() {
	engine := flag.String("engine", "google", "搜索引擎: google/youtube/wikipedia/weather/amazon/temu/reddit/bing/duckduckgo/baidu")
	query := flag.String("query", "", "搜索关键词")
	profileDir := flag.String("profile-dir", "./.chrome-profile", "Chrome/Chromium User Data 目录")
	channel := flag.String("channel", "chrome", "浏览器通道: chrome/chromium/msedge 等")
	maxResults := flag.Int("max-results", 10, "返回结果数量上限")
	timeout := flag.Duration("timeout", 90*time.Second, "总超时时间")
	headless := flag.String("headless", "false", "无头模式: true/false/first")
	snapshot := flag.Bool("snapshot", false, "是否输出搜索结果页截图")
	autoInstall := flag.Bool("install", false, "启动前自动安装 Playwright 浏览器驱动")
	flag.Parse()

	if strings.TrimSpace(*query) == "" {
		fmt.Fprintln(os.Stderr, "参数错误: --query 不能为空")
		os.Exit(2)
	}
	engineValue := strings.ToLower(strings.TrimSpace(*engine))
	if !isSupportedEngine(engineValue) {
		fmt.Fprintln(os.Stderr, "参数错误: --engine 仅支持 google/youtube/wikipedia/weather/amazon/temu/reddit/bing/duckduckgo/baidu")
		os.Exit(2)
	}
	if *maxResults <= 0 {
		fmt.Fprintln(os.Stderr, "参数错误: --max-results 必须大于 0")
		os.Exit(2)
	}
	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "参数错误: --timeout 必须大于 0")
		os.Exit(2)
	}
	headlessMode := strings.ToLower(strings.TrimSpace(*headless))
	if headlessMode != "true" && headlessMode != "false" && headlessMode != "first" {
		fmt.Fprintln(os.Stderr, "参数错误: --headless 仅支持 true/false/first")
		os.Exit(2)
	}

	if err := os.MkdirAll(*profileDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "创建 profile 目录失败: %v\n", err)
		os.Exit(1)
	}

	if *autoInstall {
		if err := installPlaywrightBrowsers(); err != nil {
			fmt.Fprintf(os.Stderr, "安装 Playwright 浏览器失败: %v\n", err)
			os.Exit(1)
		}
	}

	results, snapshotPath, err := runSearch(engineValue, *query, *profileDir, *channel, *maxResults, *timeout, headlessMode, *snapshot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "搜索失败: %v\n", err)
		os.Exit(1)
	}

	for i, result := range results {
		fmt.Printf("%d. %s\n%s\n", i+1, result.Title, result.URL)
		fmt.Printf("\n")
	}
	if strings.TrimSpace(snapshotPath) != "" {
		fmt.Printf("搜索结果页截图: %s\n", snapshotPath)
	}
}

func installPlaywrightBrowsers() error {
	if err := ensurePlaywrightTestPackage(); err != nil {
		return err
	}
	cmd := exec.Command("npx", "playwright", "install", "chromium")
	cmd.Dir = mustGetwd()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runSearch(engine, query, profileDir, channel string, maxResults int, timeout time.Duration, headlessMode string, snapshot bool) ([]SearchResult, string, error) {
	result, err := runSearchOnce(engine, query, profileDir, channel, maxResults, timeout, headlessMode, snapshot)
	if err != nil {
		return nil, "", err
	}
	if result.Status == ScriptStatusHumanVerification {
		return nil, "", errors.New("检测到人机验证，请使用 --headless false 或 --headless first")
	}
	if result.Status != ScriptStatusOK {
		return nil, "", fmt.Errorf("搜索失败: %s", result.Status)
	}
	return result.Results, result.SnapshotPath, nil
}

func runSearchOnce(engine, query, profileDir, channel string, maxResults int, timeout time.Duration, headlessMode string, snapshot bool) (*ScriptResult, error) {
	if err := ensurePlaywrightTestPackage(); err != nil {
		return nil, err
	}
	result, err := executePlaywrightScript(engine, query, profileDir, channel, maxResults, timeout, headlessMode, snapshot)
	if err != nil {
		return nil, err
	}
	switch ScriptStatus(strings.TrimSpace(string(result.Status))) {
	case ScriptStatusOK:
		if len(result.Results) == 0 {
			return nil, errors.New("未获取到搜索结果")
		}
		return result, nil
	case ScriptStatusHumanVerification:
		return result, nil
	case ScriptStatusError:
		msg := strings.TrimSpace(result.Error)
		if msg == "" {
			msg = "脚本执行失败"
		}
		return nil, errors.New(msg)
	default:
		return nil, fmt.Errorf("未知脚本状态: %s", result.Status)
	}
}

func executePlaywrightScript(engine, query, profileDir, channel string, maxResults int, timeout time.Duration, headlessMode string, snapshot bool) (*ScriptResult, error) {
	absProfileDir, err := filepath.Abs(profileDir)
	if err != nil {
		return nil, fmt.Errorf("解析 profile 路径失败: %w", err)
	}

	projectRoot := mustGetwd()
	outputFile, err := os.CreateTemp("", "pw-result-*.json")
	if err != nil {
		return nil, fmt.Errorf("创建输出文件失败: %w", err)
	}
	outputPath := outputFile.Name()
	_ = outputFile.Close()
	defer os.Remove(outputPath)

	scriptContent, err := playwrightScript(PlaywrightScriptData{
		Engine:        engine,
		Query:         query,
		ProfileDir:    absProfileDir,
		Channel:       strings.TrimSpace(channel),
		MaxResults:    maxResults,
		TimeoutMS:     timeout.Milliseconds(),
		HeadlessMode:  headlessMode,
		Snapshot:      snapshot,
		SnapshotStamp: time.Now().Format("20060102-150405"),
		ProjectRoot:   projectRoot,
		OutputPath:    outputPath,
	}, engine)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("npx", "node", "-")
	cmd.Dir = projectRoot
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(scriptContent)
	commandOutput, cmdErr := cmd.CombinedOutput()

	payloadBytes, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		if cmdErr != nil {
			return nil, fmt.Errorf("执行 playwright 命令失败: %w\n%s", cmdErr, strings.TrimSpace(string(commandOutput)))
		}
		return nil, fmt.Errorf("读取脚本输出失败: %w", readErr)
	}

	var result ScriptResult
	if err := json.Unmarshal(payloadBytes, &result); err != nil {
		return nil, fmt.Errorf("解析脚本输出失败: %w", err)
	}

	if cmdErr != nil && strings.TrimSpace(string(result.Status)) == "" {
		return nil, fmt.Errorf("执行 playwright 命令失败: %w\n%s", cmdErr, strings.TrimSpace(string(commandOutput)))
	}
	return &result, nil
}

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func ensurePlaywrightTestPackage() error {
	projectRoot := mustGetwd()
	modulePath := filepath.Join(projectRoot, "node_modules", "@playwright", "test")
	if _, err := os.Stat(modulePath); err == nil {
		return nil
	}

	packageJSONPath := filepath.Join(projectRoot, "package.json")
	if _, err := os.Stat(packageJSONPath); err != nil {
		content := "{\n  \"name\": \"agent-web-fetch\",\n  \"private\": true\n}\n"
		if writeErr := os.WriteFile(packageJSONPath, []byte(content), 0o644); writeErr != nil {
			return fmt.Errorf("创建 package.json 失败: %w", writeErr)
		}
	}

	cmd := exec.Command("npm", "install", "--save-dev", "@playwright/test")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("安装 @playwright/test 失败: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func playwrightScript(data PlaywrightScriptData, engine string) (string, error) {
	switch engine {
	case "google":
		return renderGooglePlaywrightScript(data)
	case "youtube":
		return renderYouTubePlaywrightScript(data)
	case "wikipedia":
		return renderWikipediaPlaywrightScript(data)
	case "weather":
		return renderWeatherPlaywrightScript(data)
	case "amazon":
		return renderAmazonPlaywrightScript(data)
	case "temu":
		return renderTemuPlaywrightScript(data)
	case "reddit":
		return renderRedditPlaywrightScript(data)
	case "bing":
		return renderBingPlaywrightScript(data)
	case "duckduckgo":
		return renderDuckDuckGoPlaywrightScript(data)
	case "baidu":
		return renderBaiduPlaywrightScript(data)
	default:
		return "", fmt.Errorf("不支持的搜索引擎: %s", engine)
	}
}

func isSupportedEngine(engine string) bool {
	switch engine {
	case "google", "youtube", "wikipedia", "weather", "amazon", "temu", "reddit", "bing", "duckduckgo", "baidu":
		return true
	default:
		return false
	}
}

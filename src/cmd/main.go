package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhyoulun/agent-web-fetch/src"
	"github.com/zhyoulun/agent-web-fetch/src/sites"
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

var playwrightFactory = src.NewPlaywrightFactory()

func main() {
	engine := flag.String("engine", "google", "搜索引擎: google/youtube/wikipedia/amazon/reddit/bing/duckduckgo/baidu/tiktok/douban/imdb/github/piratebay/chatgpt/grok/gemini")
	query := flag.String("query", "", "搜索关键词或问题")
	profileDir := flag.String("profile-dir", "./.chrome-profile", "Chrome/Chromium User Data 目录")
	channel := flag.String("channel", "chrome", "浏览器通道: chrome/chromium/msedge 等")
	login := flag.Bool("login", false, "允许在浏览器中等待人工完成站点登录；当前主要用于 youtube 的 Google 账号登录")
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
	if !playwrightFactory.Supports(engineValue) {
		fmt.Fprintln(os.Stderr, "参数错误: --engine 仅支持 google/youtube/wikipedia/amazon/reddit/bing/duckduckgo/baidu/tiktok/douban/imdb/github/piratebay/chatgpt/grok/gemini")
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

	results, snapshotPath, err := runSearch(engineValue, *query, *profileDir, *channel, *login, *maxResults, *timeout, headlessMode, *snapshot)
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

func runSearch(engine, query, profileDir, channel string, login bool, maxResults int, timeout time.Duration, headlessMode string, snapshot bool) ([]SearchResult, string, error) {
	if err := ensurePlaywrightTestPackage(); err != nil {
		return nil, "", err
	}
	result, err := executePlaywrightScript(engine, query, profileDir, channel, login, maxResults, timeout, headlessMode, snapshot)
	if err != nil {
		return nil, "", err
	}
	switch ScriptStatus(strings.TrimSpace(string(result.Status))) {
	case ScriptStatusOK:
		if len(result.Results) == 0 {
			return nil, "", errors.New("未获取到搜索结果")
		}
		return result.Results, result.SnapshotPath, nil
	case ScriptStatusHumanVerification:
		reason := strings.TrimSpace(result.Reason)
		if strings.HasSuffix(reason, "_login_required") {
			target := "目标站点"
			switch reason {
			case "google_login_required":
				target = "Google 账号"
			case "chatgpt_login_required":
				target = "ChatGPT"
			}
			return nil, "", fmt.Errorf("需要先登录%s，请使用 --headless false 或 --headless first，并复用同一个 --profile-dir", target)
		}
		return nil, "", errors.New("检测到人机验证，请使用 --headless false 或 --headless first")
	case ScriptStatusError:
		msg := strings.TrimSpace(result.Error)
		if msg == "" {
			msg = "脚本执行失败"
		}
		return nil, "", errors.New(msg)
	default:
		return nil, "", fmt.Errorf("未知脚本状态: %s", result.Status)
	}
}

func executePlaywrightScript(engine, query, profileDir, channel string, login bool, maxResults int, timeout time.Duration, headlessMode string, snapshot bool) (*ScriptResult, error) {
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

	scriptContent, err := playwrightFactory.Render(sites.PlaywrightScriptData{
		Engine:        engine,
		Query:         query,
		ProfileDir:    absProfileDir,
		Channel:       strings.TrimSpace(channel),
		Login:         login,
		MaxResults:    maxResults,
		TimeoutMS:     timeout.Milliseconds(),
		HeadlessMode:  headlessMode,
		Snapshot:      snapshot,
		SnapshotStamp: time.Now().Format("20060102-150405"),
		ProjectRoot:   projectRoot,
		OutputPath:    outputPath,
	})
	if err != nil {
		return nil, err
	}

	tempScriptDir := filepath.Join(projectRoot, ".tmp-playwright")
	if err := os.MkdirAll(tempScriptDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建脚本目录失败: %w", err)
	}

	scriptFile, err := os.CreateTemp(tempScriptDir, "pw-script-*.js")
	if err != nil {
		return nil, fmt.Errorf("创建脚本文件失败: %w", err)
	}
	scriptPath := scriptFile.Name()
	if _, err := scriptFile.WriteString(scriptContent); err != nil {
		_ = scriptFile.Close()
		_ = os.Remove(scriptPath)
		return nil, fmt.Errorf("写入脚本文件失败: %w", err)
	}
	if err := scriptFile.Close(); err != nil {
		_ = os.Remove(scriptPath)
		return nil, fmt.Errorf("关闭脚本文件失败: %w", err)
	}
	defer os.Remove(scriptPath)

	cmd := exec.Command("node", scriptPath)
	cmd.Dir = projectRoot
	cmd.Env = os.Environ()
	var commandOutput bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stderr, &commandOutput)
	cmd.Stderr = io.MultiWriter(os.Stderr, &commandOutput)

	fmt.Fprintf(os.Stderr, "[%s] 正在启动 Playwright 脚本...\n", engine)
	cmdErr := cmd.Run()

	payloadBytes, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		if cmdErr != nil {
			return nil, fmt.Errorf("执行 playwright 命令失败: %w\n%s", cmdErr, strings.TrimSpace(commandOutput.String()))
		}
		return nil, fmt.Errorf("读取脚本输出失败: %w", readErr)
	}

	var result ScriptResult
	if err := json.Unmarshal(payloadBytes, &result); err != nil {
		return nil, fmt.Errorf("解析脚本输出失败: %w", err)
	}

	if cmdErr != nil && strings.TrimSpace(string(result.Status)) == "" {
		return nil, fmt.Errorf("执行 playwright 命令失败: %w\n%s", cmdErr, strings.TrimSpace(commandOutput.String()))
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

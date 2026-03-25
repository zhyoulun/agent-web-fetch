package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zhyoulun/agent-web-fetch/src"
	"github.com/zhyoulun/agent-web-fetch/src/sites"
)

type SearchResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
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

type commandResponse struct {
	Status          string         `json:"status"`
	Engine          string         `json:"engine,omitempty"`
	Query           string         `json:"query,omitempty"`
	Script          string         `json:"script,omitempty"`
	Results         []SearchResult `json:"results,omitempty"`
	SnapshotPath    string         `json:"snapshotPath,omitempty"`
	Error           string         `json:"error,omitempty"`
	Installed       []string       `json:"installed,omitempty"`
	SupportedEngine []engineInfo   `json:"supportedEngines,omitempty"`
}

type engineInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type commandOptions struct {
	engine      string
	query       string
	profileDir  string
	channel     string
	login       bool
	maxResults  int
	timeout     time.Duration
	headless    string
	snapshot    bool
	autoInstall bool
	dumpScript  bool
}

type cliError struct {
	code int
	msg  string
}

func (e *cliError) Error() string {
	return e.msg
}

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		exitCode := 1
		response := commandResponse{
			Status: "error",
			Error:  err.Error(),
		}
		var commandErr *cliError
		if errors.As(err, &commandErr) {
			exitCode = commandErr.code
		}
		_ = writeJSON(os.Stdout, response)
		os.Exit(exitCode)
	}
}

func newRootCmd() *cobra.Command {
	opts := &commandOptions{
		engine:     "google",
		profileDir: "./.chrome-profile",
		channel:    "chrome",
		maxResults: 10,
		timeout:    90 * time.Second,
		headless:   "false",
	}

	cmd := &cobra.Command{
		Use:           "agent-web-fetch",
		Short:         "使用 Playwright 驱动浏览器执行站点搜索或 AI 问答",
		Long:          buildRootLongHelp(),
		Example:       buildRootExamples(),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(opts)
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &cliError{code: 2, msg: fmt.Sprintf("参数错误: %v", err)}
	})

	flags := cmd.Flags()
	flags.StringVar(&opts.engine, "engine", opts.engine, "目标引擎，使用 engines 子命令可查看完整列表")
	flags.StringVar(&opts.query, "query", opts.query, "搜索关键词或提问内容")
	flags.StringVar(&opts.profileDir, "profile-dir", opts.profileDir, "Chrome/Chromium 用户数据目录，用于复用 Cookie、登录态和验证状态")
	flags.StringVar(&opts.channel, "channel", opts.channel, "浏览器通道，例如 chrome、chromium、msedge")
	flags.BoolVar(&opts.login, "login", opts.login, "允许在浏览器中等待人工完成站点登录；当前主要用于 YouTube 的 Google 账号登录")
	flags.IntVar(&opts.maxResults, "max-results", opts.maxResults, "返回结果数量上限")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "单次任务总超时时间，例如 30s、2m")
	flags.StringVar(&opts.headless, "headless", opts.headless, "浏览器模式：true、false、first；部分引擎会忽略该值并强制使用可见浏览器")
	flags.BoolVar(&opts.snapshot, "snapshot", opts.snapshot, "保存结果页截图到 snapshots/<timestamp>/")
	flags.BoolVar(&opts.autoInstall, "install", opts.autoInstall, "运行前自动安装 Playwright Chromium 依赖")
	flags.BoolVar(&opts.dumpScript, "dump-script", opts.dumpScript, "输出渲染后的 Playwright 脚本内容，不执行浏览器")

	cmd.AddCommand(newEnginesCmd())
	return cmd
}

func newEnginesCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "engines",
		Short:         "列出当前支持的搜索引擎和站点",
		Long:          "列出当前可用的引擎名称，以及它们更适合的使用场景。",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			response := commandResponse{
				Status:          "ok",
				SupportedEngine: supportedEngines(),
			}
			_ = writeJSON(cmd.OutOrStdout(), response)
		},
	}
}

func runCommand(opts *commandOptions) error {
	engineValue, headlessMode, err := validateCommandOptions(opts)
	if err != nil {
		return err
	}
	var installed []string

	if err := os.MkdirAll(opts.profileDir, 0o755); err != nil {
		return fmt.Errorf("创建 profile 目录失败: %w", err)
	}

	if opts.autoInstall {
		installed, err = installPlaywrightBrowsers()
		if err != nil {
			return fmt.Errorf("安装 Playwright 浏览器失败: %w", err)
		}
	}

	if opts.dumpScript {
		scriptContent, err := renderPlaywrightScript(engineValue, opts.query, opts.profileDir, opts.channel, opts.login, opts.maxResults, opts.timeout, headlessMode, opts.snapshot, scriptPreviewOutputPath())
		if err != nil {
			return fmt.Errorf("生成 Playwright 脚本失败: %w", err)
		}
		response := commandResponse{
			Status:    "ok",
			Engine:    engineValue,
			Query:     opts.query,
			Script:    scriptContent,
			Installed: installed,
		}
		return writeJSON(os.Stdout, response)
	}

	results, snapshotPath, err := runSearch(engineValue, opts.query, opts.profileDir, opts.channel, opts.login, opts.maxResults, opts.timeout, headlessMode, opts.snapshot)
	if err != nil {
		return fmt.Errorf("搜索失败: %w", err)
	}
	response := commandResponse{
		Status:       "ok",
		Engine:       engineValue,
		Query:        opts.query,
		Results:      results,
		SnapshotPath: strings.TrimSpace(snapshotPath),
		Installed:    installed,
	}
	return writeJSON(os.Stdout, response)
}

func validateCommandOptions(opts *commandOptions) (string, string, error) {
	if strings.TrimSpace(opts.query) == "" {
		return "", "", &cliError{code: 2, msg: "参数错误: --query 不能为空"}
	}

	engineValue := strings.ToLower(strings.TrimSpace(opts.engine))
	if !playwrightFactory.Supports(engineValue) {
		return "", "", &cliError{code: 2, msg: fmt.Sprintf("参数错误: --engine 仅支持 %s", supportedEngineList())}
	}

	if opts.maxResults <= 0 {
		return "", "", &cliError{code: 2, msg: "参数错误: --max-results 必须大于 0"}
	}
	if opts.timeout <= 0 {
		return "", "", &cliError{code: 2, msg: "参数错误: --timeout 必须大于 0"}
	}

	headlessMode := strings.ToLower(strings.TrimSpace(opts.headless))
	if headlessMode != "true" && headlessMode != "false" && headlessMode != "first" {
		return "", "", &cliError{code: 2, msg: "参数错误: --headless 仅支持 true/false/first"}
	}

	return engineValue, headlessMode, nil
}

func supportedEngineList() string {
	return strings.Join(playwrightFactory.Engines(), "/")
}

func supportedEngines() []engineInfo {
	engines := playwrightFactory.Engines()
	items := make([]engineInfo, 0, len(engines))
	for _, engine := range engines {
		items = append(items, engineInfo{
			Name:        engine,
			Description: engineDescription(engine),
		})
	}
	return items
}

func buildRootLongHelp() string {
	lines := []string{
		"通过 Playwright 启动真实浏览器，在指定站点执行搜索或 AI 问答。",
		"",
		"适用场景：",
		"  - 网页搜索站点结果抓取，例如 Google、GitHub、IMDb",
		"  - 需要登录态或人机验证的站点，配合 --profile-dir 复用浏览器会话",
		"  - AI 站点提问，例如 ChatGPT、Gemini、Grok",
		"",
		"当前支持的引擎：",
	}
	for _, engine := range playwrightFactory.Engines() {
		lines = append(lines, fmt.Sprintf("  %-10s %s", engine, engineDescription(engine)))
	}
	lines = append(lines,
		"",
		"补充说明：",
		"  - --profile-dir 建议固定复用，这样登录态和验证状态可以持续保留。",
		"  - --headless first 会先尝试无头模式；若站点要求人工处理，部分引擎会退回可见浏览器。",
		"  - 某些引擎会强制使用可见浏览器，即使传入了 --headless true。",
	)
	return strings.Join(lines, "\n")
}

func buildRootExamples() string {
	return strings.Join([]string{
		"  查看支持的引擎：",
		"    agent-web-fetch engines",
		"",
		"  GitHub 仓库搜索：",
		"    agent-web-fetch --engine github --query playwright --max-results 5",
		"",
		"  Google 网页搜索：",
		"    agent-web-fetch --engine google --query \"site:golang.org cobra\" --max-results 5",
		"",
		"  豆瓣电影搜索：",
		"    agent-web-fetch --engine douban --query \"阿凡达\" --max-results 5",
		"",
		"  Pirate Bay 搜索：",
		"    agent-web-fetch --engine piratebay --query ubuntu --profile-dir .chrome-profile --channel chrome",
		"",
		"  ChatGPT 提问，并复用浏览器登录态：",
		"    agent-web-fetch --engine chatgpt --query \"悉尼天气\" --profile-dir .chrome-profile --channel chrome --headless false",
		"",
		"  先安装 Playwright 浏览器，再执行搜索：",
		"    agent-web-fetch --install --engine github --query playwright",
		"",
		"  只导出渲染后的 Playwright 脚本，不执行浏览器：",
		"    agent-web-fetch --engine github --query playwright --dump-script",
	}, "\n")
}

func engineDescription(engine string) string {
	descriptions := map[string]string{
		"amazon":     "Amazon 商品搜索",
		"baidu":      "百度网页搜索",
		"bing":       "Bing 网页搜索",
		"chatgpt":    "ChatGPT 提问",
		"douban":     "豆瓣电影搜索",
		"duckduckgo": "DuckDuckGo 网页搜索",
		"gemini":     "Gemini 提问",
		"github":     "GitHub 仓库搜索",
		"google":     "Google 网页搜索",
		"grok":       "Grok 提问",
		"imdb":       "IMDb 条目搜索",
		"piratebay":  "Pirate Bay 代理站搜索",
		"reddit":     "Reddit 搜索",
		"tiktok":     "TikTok 站内搜索",
		"wikipedia":  "Wikipedia 条目搜索",
		"youtube":    "YouTube 搜索",
	}
	if desc, ok := descriptions[engine]; ok {
		return desc
	}
	return "未分类引擎"
}

func installPlaywrightBrowsers() ([]string, error) {
	if err := ensurePlaywrightTestPackage(); err != nil {
		return nil, err
	}

	browsers, extraArgs, err := playwrightInstallPlan()
	if err != nil {
		return nil, err
	}

	args := []string{"playwright", "install"}
	args = append(args, extraArgs...)
	args = append(args, browsers...)

	cmd := exec.Command("npx", args...)
	cmd.Dir = mustGetwd()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(output)))
	}
	return browsers, nil
}

func playwrightInstallPlan() ([]string, []string, error) {
	switch runtime.GOOS {
	case "darwin", "windows":
		return []string{"chromium", "firefox", "webkit"}, nil, nil
	case "linux":
		return []string{"chromium", "firefox", "webkit"}, []string{"--with-deps"}, nil
	default:
		return nil, nil, fmt.Errorf("当前平台暂不支持自动安装 Playwright 浏览器: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
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
	projectRoot := mustGetwd()
	outputFile, err := os.CreateTemp("", "pw-result-*.json")
	if err != nil {
		return nil, fmt.Errorf("创建输出文件失败: %w", err)
	}
	outputPath := outputFile.Name()
	_ = outputFile.Close()
	defer os.Remove(outputPath)

	scriptContent, err := renderPlaywrightScript(engine, query, profileDir, channel, login, maxResults, timeout, headlessMode, snapshot, outputPath)
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

func renderPlaywrightScript(engine, query, profileDir, channel string, login bool, maxResults int, timeout time.Duration, headlessMode string, snapshot bool, outputPath string) (string, error) {
	absProfileDir, err := filepath.Abs(profileDir)
	if err != nil {
		return "", fmt.Errorf("解析 profile 路径失败: %w", err)
	}

	projectRoot := mustGetwd()
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
		return "", err
	}
	return scriptContent, nil
}

func scriptPreviewOutputPath() string {
	return filepath.Join(mustGetwd(), ".tmp-playwright", "pw-result-preview.json")
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

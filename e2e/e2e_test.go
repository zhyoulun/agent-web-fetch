//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type cliCase struct {
	site  string
	query string
}

type pageProbeCase struct {
	site  string
	query string
	url   string
}

type probeResult struct {
	Title             string              `json:"title"`
	FinalURL          string              `json:"finalUrl"`
	BodyPreview       string              `json:"bodyPreview"`
	Results           []SearchProbeResult `json:"results"`
	HumanIntervention bool                `json:"humanIntervention"`
}

type SearchProbeResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

const pageProbeRunnerScript = `
const { chromium } = require('@playwright/test');
const humanVerificationTimeoutMs = 10 * 60 * 1000;

function detectHumanVerification(url, bodyText) {
  const lowURL = String(url || '').toLowerCase();
  const urlMarkers = ['/sorry/', 'recaptcha', 'captcha', 'challenge', 'interstitial', 'anomaly'];
  for (const marker of urlMarkers) {
    if (lowURL.includes(marker)) return marker;
  }

  const body = String(bodyText || '').toLowerCase();
  const keywords = [
    'verify you are human',
    "i'm not a robot",
    'unusual traffic',
    'detected unusual traffic',
    'complete the following challenge',
    'please complete the following challenge',
    'prove you are human',
    'captcha',
    '请完成以下验证',
    '请完成以下挑战',
    '异常流量',
    '机器人也使用 duckduckgo',
    '确认这项搜索由真人进行',
    '继续之前',
    '最后一步',
    '请解决以下难题以继续',
  ];
  for (const keyword of keywords) {
    if (body.includes(keyword.toLowerCase())) return keyword;
  }
  return '';
}

async function readBodyText(page) {
  return await page.locator('body').innerText({ timeout: 5000 }).catch(() => '');
}

async function launchContext(profileDir, headless) {
  return await chromium.launchPersistentContext(profileDir, {
    channel: 'chrome',
    headless,
    userAgent:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
    viewport: { width: 1440, height: 1600 },
    locale: 'zh-CN',
  });
}

async function openAndResolvePage(url, profileDir) {
  let context = await launchContext(profileDir, true);
  let page = await context.newPage();

  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 45000 });
  await page.waitForTimeout(5000);

  let bodyText = await readBodyText(page);
  const firstReason = detectHumanVerification(page.url(), bodyText);
  if (!firstReason) {
    return { context, page, bodyText, humanIntervention: false };
  }

  await context.close().catch(() => {});

  context = await launchContext(profileDir, false);
  page = await context.newPage();
  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 45000 }).catch(() => {});
  await page.bringToFront().catch(() => {});
  process.stderr.write('检测到人机验证，已打开浏览器窗口，请手动完成验证，完成后测试会自动继续。\n');

  const deadline = Date.now() + humanVerificationTimeoutMs;
  while (Date.now() < deadline) {
    bodyText = await readBodyText(page);
    const reason = detectHumanVerification(page.url(), bodyText);
    if (!reason) {
      await page.waitForTimeout(3000);
      bodyText = await readBodyText(page);
      process.stderr.write('人工验证已完成，继续采集结果。\n');
      return { context, page, bodyText, humanIntervention: true };
    }
    await page.waitForTimeout(1200);
  }

  throw new Error('等待人工完成人机验证超时');
}

async function extractRedditResults(context, searchURL) {
  const url = new URL(searchURL);
  const query = url.searchParams.get('q') || '';
  if (!query) return [];

  const page = await context.newPage();
  try {
    await page.goto('https://old.reddit.com/search?q=' + encodeURIComponent(query) + '&type=link', {
      waitUntil: 'domcontentloaded',
      timeout: 45000,
    });
    await page.waitForTimeout(3000);
    return await page.evaluate(() => {
      return Array.from(document.querySelectorAll('.search-result.search-result-link'))
        .slice(0, 5)
        .map((item) => {
          const link = item.querySelector('a.search-title');
          return {
            title: String(link?.textContent || '').replace(/\s+/g, ' ').trim(),
            url: String(link?.href || '').trim(),
          };
        })
        .filter((item) => item.title && item.url);
    });
  } finally {
    await page.close().catch(() => {});
  }
}

async function extractDuckDuckGoResults(context, searchURL) {
  const page = await context.newPage();
  try {
    await page.goto(searchURL, {
      waitUntil: 'domcontentloaded',
      timeout: 45000,
    });
    await page.waitForTimeout(3000);
    return await page.evaluate(() => {
      const normalize = (raw) => {
        if (!raw) return '';
        let parsed;
        try {
          parsed = new URL(String(raw).trim(), 'https://duckduckgo.com');
        } catch {
          return '';
        }
        if (!['http:', 'https:'].includes(parsed.protocol)) return '';
        const host = parsed.hostname.toLowerCase();
        if (host.endsWith('duckduckgo.com') || host.endsWith('duck.com')) {
          const target = parsed.searchParams.get('uddg');
          return target ? normalize(target) : '';
        }
        parsed.hash = '';
        return parsed.toString();
      };

      const seen = new Set();
      return Array.from(document.querySelectorAll('[data-testid="result"]'))
        .map((item) => {
          const link =
            item.querySelector('a[data-testid="result-title-a"]') ||
            item.querySelector('h2 a');
          const url = normalize(link?.getAttribute('href') || '');
          return {
            title: String(link?.textContent || '').replace(/\s+/g, ' ').trim(),
            url,
          };
        })
        .filter((item) => {
          if (!item.title || !item.url) return false;
          if (seen.has(item.url)) return false;
          seen.add(item.url);
          return true;
        })
        .slice(0, 5);
    });
  } finally {
    await page.close().catch(() => {});
  }
}

async function extractAmazonResults(context, searchURL) {
  const page = await context.newPage();
  try {
    await page.goto(searchURL, {
      waitUntil: 'domcontentloaded',
      timeout: 45000,
    });
    await page.waitForTimeout(5000);
    return await page.evaluate(() => {
      const normalize = (raw) => {
        if (!raw) return '';
        let parsed;
        try {
          parsed = new URL(String(raw).trim(), location.href);
        } catch {
          return '';
        }
        if (!['http:', 'https:'].includes(parsed.protocol)) return '';
        const host = parsed.hostname.toLowerCase();
        if (!host.includes('amazon.')) return '';
        if (!parsed.pathname.includes('/dp/') && !parsed.pathname.includes('/gp/')) return '';
        parsed.hash = '';
        return parsed.toString();
      };

      return Array.from(document.querySelectorAll('[data-component-type="s-search-result"]'))
        .map((item) => {
          const titleLink =
            item.querySelector('[data-cy="title-recipe"] a') ||
            item.querySelector('h2 a.a-link-normal[href*="/dp/"]') ||
            item.querySelector('a[href*="/dp/"], a[href*="/gp/"]');
          const title = String(titleLink?.textContent || '').replace(/\s+/g, ' ').trim();
          const url = normalize(titleLink?.getAttribute('href') || '');
          return { title, url };
        })
        .filter((item) => item.title && item.url)
        .slice(0, 5);
    });
  } finally {
    await page.close().catch(() => {});
  }
}

async function extractGoogleResults(context, searchURL) {
  const page = await context.newPage();
  try {
    await page.goto(searchURL, {
      waitUntil: 'domcontentloaded',
      timeout: 45000,
    });
    await page.waitForTimeout(3000);
    return await page.evaluate(() => {
      const blockedControllers = new Set(['JnUebe', 'tIhjPc']);

      const isGoogleHost = (hostname) => {
        const host = String(hostname || '').trim().toLowerCase();
        return host === 'google.com' || host.endsWith('.google.com');
      };

      const normalize = (raw) => {
        if (!raw) return '';
        let parsed;
        try {
          parsed = new URL(String(raw).trim().replaceAll('&amp;', '&'), 'https://www.google.com');
        } catch {
          return '';
        }
        if (!['http:', 'https:'].includes(parsed.protocol)) return '';

        for (let i = 0; i < 5; i++) {
          const host = parsed.hostname.toLowerCase();
          const pathname = parsed.pathname.toLowerCase();

          if (isGoogleHost(host) && (pathname === '/url' || pathname === '/imgres')) {
            const target = parsed.searchParams.get('q') || parsed.searchParams.get('url') || parsed.searchParams.get('imgurl');
            if (!target) return '';
            try {
              parsed = new URL(target, parsed);
            } catch {
              return '';
            }
            if (!['http:', 'https:'].includes(parsed.protocol)) return '';
            continue;
          }

          if (isGoogleHost(host)) {
            if (
              pathname === '/' ||
              pathname === '/search' ||
              pathname === '/sorry/index' ||
              pathname === '/sorry/' ||
              pathname === '/setprefs' ||
              pathname.startsWith('/search') ||
              pathname.startsWith('/accounts')
            ) {
              return '';
            }
          }

          parsed.hash = '';
          return parsed.toString();
        }

        return '';
      };

      const results = [];
      const seen = new Set();
      for (const item of Array.from(document.querySelectorAll('#search h3'))) {
        if (results.length >= 5) break;
        const controller = item.closest('[jscontroller]')?.getAttribute('jscontroller') || '';
        if (blockedControllers.has(controller)) continue;
        const title = String(item.textContent || '').replace(/\s+/g, ' ').trim();
        if (!title) continue;
        const anchor = item.closest('a') || item.parentElement?.closest('a');
        const url = normalize(anchor?.getAttribute('href') || '');
        if (!url || seen.has(url)) continue;
        seen.add(url);
        results.push({ title, url });
      }
      return results;
    });
  } finally {
    await page.close().catch(() => {});
  }
}

async function extractBingResults(context, searchURL) {
  const page = await context.newPage();
  try {
    await page.goto(searchURL, {
      waitUntil: 'domcontentloaded',
      timeout: 45000,
    });
    await page.waitForTimeout(3000);
    return await page.evaluate(() => {
      const tryDecodeBingRedirect = (value) => {
        if (!value || !value.startsWith('a1')) return '';
        try {
          const text = atob(value.slice(2));
          if (text.startsWith('http://') || text.startsWith('https://')) return text;
        } catch {}
        return '';
      };

      const normalize = (raw) => {
        if (!raw) return '';
        let parsed;
        try {
          parsed = new URL(String(raw).trim(), 'https://www.bing.com');
        } catch {
          return '';
        }
        if (!['http:', 'https:'].includes(parsed.protocol)) return '';
        const host = parsed.hostname.toLowerCase();
        if (host.endsWith('bing.com') && parsed.pathname.startsWith('/ck/a')) {
          const decoded = tryDecodeBingRedirect(parsed.searchParams.get('u') || '');
          if (decoded) return decoded;
        }
        parsed.hash = '';
        return parsed.toString();
      };

      const results = [];
      const seen = new Set();
      for (const item of Array.from(document.querySelectorAll('li.b_algo h2 a, h2 a'))) {
        if (results.length >= 5) break;
        const title = String(item.textContent || '').replace(/\s+/g, ' ').trim();
        const url = normalize(item.getAttribute('href') || '');
        if (!title || !url || seen.has(url)) continue;
        seen.add(url);
        results.push({ title, url });
      }
      return results;
    });
  } finally {
    await page.close().catch(() => {});
  }
}

async function extractTikTokResults(context, searchURL) {
  const page = await context.newPage();
  try {
    await page.goto(searchURL, {
      waitUntil: 'domcontentloaded',
      timeout: 45000,
    });
    await page.waitForTimeout(4000);
    return await page.evaluate(() => {
      const normalizeLink = (raw) => {
        if (!raw) return '';
        let parsed;
        try {
          parsed = new URL(String(raw).trim(), 'https://www.tiktok.com');
        } catch {
          return '';
        }
        if (!['http:', 'https:'].includes(parsed.protocol)) return '';
        const host = parsed.hostname.toLowerCase();
        if (host !== 'www.tiktok.com' && host !== 'tiktok.com' && !host.endsWith('.tiktok.com')) return '';
        if (!parsed.pathname.includes('/video/')) return '';
        parsed.search = '';
        parsed.hash = '';
        return parsed.toString();
      };

      const normalizeTitle = (raw) => {
        const title = String(raw || '').replace(/\s+/g, ' ').trim();
        if (!title) return '';
        const lower = title.toLowerCase();
        for (const marker of ['创作的 ', 'created by ', 'posted by ']) {
          const index = lower.lastIndexOf(marker.toLowerCase());
          if (index >= 0) {
            const trimmed = title.slice(index + marker.length).trim();
            if (trimmed) return trimmed;
          }
        }
        return title;
      };

      const readText = (node, selectors) => {
        for (const selector of selectors) {
          const element = node.querySelector(selector);
          if (!element) continue;
          const text = String(
            element.getAttribute && element.getAttribute('alt')
              ? element.getAttribute('alt')
              : (element.textContent || '')
          ).trim();
          if (text) return text;
        }
        return '';
      };

      const results = [];
      const seen = new Set();
      for (const item of Array.from(document.querySelectorAll('[data-e2e="search_top-item"]'))) {
        if (results.length >= 5) break;
        const link = item.querySelector('a[href*="/video/"]');
        const url = normalizeLink(link?.getAttribute('href') || '');
        const title = normalizeTitle(
          readText(item, [
            'img[alt]',
            '[data-e2e="search-card-video-caption"]',
            '[data-e2e="search-card-desc"]',
            '[data-e2e="search-card-user-unique-id"]',
          ]),
        );
        if (!title || !url || seen.has(url)) continue;
        seen.add(url);
        results.push({ title, url });
      }
      return results;
    });
  } finally {
    await page.close().catch(() => {});
  }
}

async function main() {
  const [, url, screenshotPath, profileDir] = process.argv;
  if (!url || !screenshotPath || !profileDir) {
    throw new Error('usage: node -e <script> <url> <screenshotPath> <profileDir>');
  }

  let context;
  let page;
  let bodyText = '';
  let humanIntervention = false;

  try {
    const opened = await openAndResolvePage(url, profileDir);
    context = opened.context;
    page = opened.page;
    bodyText = opened.bodyText;
    humanIntervention = opened.humanIntervention;
    const title = await page.title();
    const finalUrl = page.url();
    const bodyPreview = bodyText.replace(/\s+/g, ' ').trim().slice(0, 1200);
    let results = [];

    if (finalUrl.includes('reddit.com/search')) {
      results = await extractRedditResults(context, finalUrl);
    }
    if (finalUrl.includes('google.')) {
      results = await extractGoogleResults(context, finalUrl);
    }
    if (finalUrl.includes('bing.com/')) {
      results = await extractBingResults(context, finalUrl);
    }
    if (finalUrl.includes('duckduckgo.com/')) {
      results = await extractDuckDuckGoResults(context, finalUrl);
    }
    if (finalUrl.includes('amazon.')) {
      results = await extractAmazonResults(context, finalUrl);
    }
    if (finalUrl.includes('tiktok.com/search')) {
      results = await extractTikTokResults(context, finalUrl);
    }

    await page.screenshot({ path: screenshotPath, fullPage: true, timeout: 15000 });

    process.stdout.write(
      JSON.stringify({
        title,
        finalUrl,
        bodyPreview,
        results,
        humanIntervention,
      }),
    );
  } finally {
    if (context) {
      await context.close();
    }
  }
}

main().catch((error) => {
  const message = String(error && error.message ? error.message : error);
  process.stderr.write(message + '\n');
  process.exit(1);
});
`

var artifactRootState struct {
	once sync.Once
	path string
	err  error
}

func TestSearchCLIStableSites(t *testing.T) {
	requireE2EEnabled(t)

	cases := []cliCase{
		{site: "youtube", query: "悉尼"},
		{site: "wikipedia", query: "南北朝"},
		{site: "baidu", query: "南北朝"},
	}

	for _, tc := range cases {
		t.Run(tc.site, func(t *testing.T) {
			siteDir := mustSiteDir(t, tc.site)
			writeTextFile(t, filepath.Join(siteDir, "query.txt"), tc.query+"\n")

			profileDir := sharedProfileDir(t)

			cmd := exec.Command(
				"go", "run", "./src/cmd",
				"--engine", tc.site,
				"--query", tc.query,
				"--profile-dir", profileDir,
				"--channel", "chrome",
				"--headless", "first",
				"--snapshot",
				"--max-results", "5",
				"--timeout", "75s",
			)
			cmd.Dir = repoRoot(t)
			output, err := cmd.CombinedOutput()
			writeTextFile(t, filepath.Join(siteDir, "result.txt"), string(output))

			meta := []string{
				"site=" + tc.site,
				"query=" + tc.query,
				"mode=cli",
				"channel=chrome",
				"command=go run ./src/cmd",
			}

			if err != nil {
				writeTextFile(t, filepath.Join(siteDir, "error.txt"), err.Error()+"\n")
				writeTextFile(t, filepath.Join(siteDir, "meta.txt"), strings.Join(meta, "\n")+"\n")
				t.Fatalf("cli probe failed: %v\n%s", err, string(output))
			}

			writeTextFile(t, filepath.Join(siteDir, "error.txt"), "")

			snapshotPath := parseSnapshotPath(string(output))
			if snapshotPath == "" {
				writeTextFile(t, filepath.Join(siteDir, "meta.txt"), strings.Join(meta, "\n")+"\n")
				t.Fatalf("missing snapshot path in output")
			}
			if _, err := os.Stat(snapshotPath); err != nil {
				writeTextFile(t, filepath.Join(siteDir, "meta.txt"), strings.Join(meta, "\n")+"\n")
				t.Fatalf("snapshot file not found: %v", err)
			}
			copyFile(t, snapshotPath, filepath.Join(siteDir, "screenshot.png"))

			if !strings.Contains(string(output), "1. ") {
				writeTextFile(t, filepath.Join(siteDir, "meta.txt"), strings.Join(meta, "\n")+"\n")
				t.Fatalf("missing numbered search result in output")
			}

			meta = append(meta, "snapshot_source="+snapshotPath)
			writeTextFile(t, filepath.Join(siteDir, "meta.txt"), strings.Join(meta, "\n")+"\n")
			assertSiteArtifacts(t, siteDir)
		})
	}
}

func TestSearchPageProbeSites(t *testing.T) {
	requireE2EEnabled(t)

	cases := []pageProbeCase{
		{site: "google", query: "悉尼", url: "https://www.google.com/search?q=%E6%82%89%E5%B0%BC"},
		{site: "amazon", query: "Vacuum", url: "https://www.amazon.com/s?k=Vacuum"},
		{site: "reddit", query: "Vacuum", url: "https://www.reddit.com/search/?q=Vacuum"},
		{site: "bing", query: "悉尼", url: "https://www.bing.com/search?q=%E6%82%89%E5%B0%BC"},
		{site: "duckduckgo", query: "Vacuum", url: "https://duckduckgo.com/?q=Vacuum&ia=web"},
		{site: "tiktok", query: "sydney", url: "https://www.tiktok.com/search?q=sydney"},
	}

	for _, tc := range cases {
		t.Run(tc.site, func(t *testing.T) {
			siteDir := mustSiteDir(t, tc.site)
			screenshotPath := filepath.Join(siteDir, "screenshot.png")
			profileDir := sharedProfileDir(t)

			writeTextFile(t, filepath.Join(siteDir, "query.txt"), tc.query+"\n")

			cmd := exec.Command("node", "-e", pageProbeRunnerScript, tc.url, screenshotPath, profileDir)
			cmd.Dir = repoRoot(t)
			var stdoutBuf bytes.Buffer
			var stderrBuf bytes.Buffer
			cmd.Stdout = &stdoutBuf
			cmd.Stderr = io.MultiWriter(&stderrBuf, os.Stderr)
			err := cmd.Run()
			output := stdoutBuf.Bytes()
			stderrOutput := stderrBuf.String()
			if err != nil {
				writeTextFile(t, filepath.Join(siteDir, "result.txt"), "")
				writeTextFile(t, filepath.Join(siteDir, "error.txt"), stderrOutput)
				writeTextFile(t, filepath.Join(siteDir, "meta.txt"), strings.Join([]string{
					"site=" + tc.site,
					"query=" + tc.query,
					"mode=page_probe",
					"url=" + tc.url,
				}, "\n")+"\n")
				t.Fatalf("page probe failed: %v\n%s", err, stderrOutput)
			}

			var result probeResult
			if err := json.Unmarshal(output, &result); err != nil {
				writeTextFile(t, filepath.Join(siteDir, "result.txt"), string(output))
				writeTextFile(t, filepath.Join(siteDir, "error.txt"), err.Error()+"\n")
				t.Fatalf("decode page probe output: %v", err)
			}

			if _, err := os.Stat(screenshotPath); err != nil {
				writeTextFile(t, filepath.Join(siteDir, "error.txt"), err.Error()+"\n")
				t.Fatalf("missing screenshot: %v", err)
			}
			if strings.TrimSpace(result.FinalURL) == "" {
				t.Fatalf("empty final URL")
			}
			if strings.TrimSpace(result.Title) == "" && strings.TrimSpace(result.BodyPreview) == "" {
				t.Fatalf("page probe returned no title and no body preview")
			}

			resultLines := []string{
				"fallback=manual_playwright",
				"site=" + tc.site,
				"query=" + tc.query,
				"title=" + result.Title,
				"final_url=" + result.FinalURL,
			}
			if len(result.Results) > 0 {
				resultLines = append(resultLines, "", "results:")
				for i, item := range result.Results {
					resultLines = append(resultLines, fmt.Sprintf("%d. %s", i+1, item.Title))
					resultLines = append(resultLines, item.URL, "")
				}
			} else {
				resultLines = append(resultLines, "", "body_preview:", result.BodyPreview)
			}
			writeTextFile(t, filepath.Join(siteDir, "result.txt"), strings.Join(resultLines, "\n")+"\n")
			writeTextFile(t, filepath.Join(siteDir, "error.txt"), "")
			humanIntervention := "false"
			if result.HumanIntervention {
				humanIntervention = "true"
			}
			writeTextFile(t, filepath.Join(siteDir, "meta.txt"), strings.Join([]string{
				"site=" + tc.site,
				"query=" + tc.query,
				"mode=page_probe",
				"channel=chrome",
				"url=" + tc.url,
				"screenshot_path=" + screenshotPath,
				"human_intervention=" + humanIntervention,
			}, "\n")+"\n")
			assertSiteArtifacts(t, siteDir)
		})
	}
}

func requireE2EEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_E2E") != "1" {
		t.Skip("set RUN_E2E=1 to execute external-site e2e tests")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func artifactRoot(t *testing.T) string {
	t.Helper()

	artifactRootState.once.Do(func() {
		stamp := time.Now().Format("20060102-150405")
		artifactRootState.path = filepath.Join(repoRoot(t), "temp", "e2e", stamp)
		artifactRootState.err = os.MkdirAll(artifactRootState.path, 0o755)
	})
	if artifactRootState.err != nil {
		t.Fatalf("create artifact root: %v", artifactRootState.err)
	}
	return artifactRootState.path
}

func sharedProfileDir(t *testing.T) string {
	t.Helper()

	profileDir := filepath.Join(repoRoot(t), ".chrome-profile")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("create shared profile dir: %v", err)
	}
	return profileDir
}

func mustSiteDir(t *testing.T, site string) string {
	t.Helper()

	siteDir := filepath.Join(artifactRoot(t), site)
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatalf("create site dir: %v", err)
	}
	return siteDir
}

func parseSnapshotPath(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "搜索结果页截图: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "搜索结果页截图: "))
		}
	}
	return ""
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read file %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write file %s: %v", dst, err)
	}
}

func assertSiteArtifacts(t *testing.T, siteDir string) {
	t.Helper()

	requiredFiles := []string{
		"query.txt",
		"result.txt",
		"meta.txt",
		"error.txt",
		"screenshot.png",
	}
	for _, name := range requiredFiles {
		path := filepath.Join(siteDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("missing artifact %s: %v", path, err)
		}
		if info.IsDir() {
			t.Fatalf("artifact is a directory, expected file: %s", path)
		}
	}
}

func TestE2EArtifactReadme(t *testing.T) {
	requireE2EEnabled(t)

	readmePath := filepath.Join(artifactRoot(t), "README.txt")
	content := strings.Join([]string{
		"Artifact root: " + artifactRoot(t),
		"",
		"CLI smoke sites:",
		"- youtube: 悉尼",
		"- wikipedia: 南北朝",
		"- baidu: 南北朝",
		"",
		"Page probe sites:",
		"- google: 悉尼",
		"- amazon: Vacuum",
		"- reddit: Vacuum",
		"- bing: 悉尼",
		"- duckduckgo: Vacuum",
		"- tiktok: sydney",
	}, "\n") + "\n"

	if err := os.WriteFile(readmePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write e2e readme: %v", err)
	}

	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("stat e2e readme: %v", err)
	}
}

func Example_e2eRunCommand() {
	fmt.Println("RUN_E2E=1 go test -tags e2e ./e2e -v")
	// Output:
	// RUN_E2E=1 go test -tags e2e ./e2e -v
}

package browser

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/cookies"
)

type browserConfig struct {
	binPath string
}

type Option func(*browserConfig)

type Browser struct {
	browser  *rod.Browser
	launcher *launcher.Launcher
	isRemote bool
}

var remoteShared struct {
	mu  sync.Mutex
	url string
	rod *rod.Browser
}

func WithBinPath(binPath string) Option {
	return func(c *browserConfig) {
		c.binPath = binPath
	}
}

// maskProxyCredentials masks username and password in proxy URL for safe logging.
func maskProxyCredentials(proxyURL string) string {
	u, err := url.Parse(proxyURL)
	if err != nil || u.User == nil {
		return proxyURL
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		u.User = url.UserPassword("***", "***")
	} else {
		u.User = url.User("***")
	}
	return u.String()
}

func remoteBrowserURL() string {
	if v := strings.TrimSpace(os.Getenv("CDP_URL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("REMOTE_BROWSER_URL")); v != "" {
		return v
	}
	return ""
}

func resolveControlURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "ws://") || strings.HasPrefix(raw, "wss://") {
		return raw
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "http://" + raw
	}
	return launcher.MustResolveURL(raw)
}

func loadCookiesOption() string {
	cookiePath := cookies.GetCookiesFilePath()
	cookieLoader := cookies.NewLoadCookie(cookiePath)
	if data, err := cookieLoader.LoadCookies(); err == nil {
		logrus.Debugf("loaded cookies from file successfully")
		return string(data)
	}
	return ""
}

func applyCookies(b *rod.Browser, cookiesJSON string) {
	if cookiesJSON == "" {
		return
	}
	var networkCookies []*proto.NetworkCookie
	if err := json.Unmarshal([]byte(cookiesJSON), &networkCookies); err != nil {
		logrus.Warnf("failed to unmarshal cookies: %v", err)
		return
	}
	b.MustSetCookies(networkCookies...)
}

func NewBrowser(headless bool, options ...Option) *Browser {
	cfg := &browserConfig{}
	for _, opt := range options {
		opt(cfg)
	}

	cookiesJSON := loadCookiesOption()

	if remote := remoteBrowserURL(); remote != "" {
		controlURL := resolveControlURL(remote)
		remoteShared.mu.Lock()
		defer remoteShared.mu.Unlock()

		if remoteShared.rod == nil || remoteShared.url != controlURL {
			remoteShared.rod = rod.New().ControlURL(controlURL).MustConnect()
			remoteShared.url = controlURL
			logrus.Infof("connected to remote browser via CDP: %s", remote)
		}
		applyCookies(remoteShared.rod, cookiesJSON)
		return &Browser{browser: remoteShared.rod, isRemote: true}
	}

	l := launcher.New().Headless(headless).Set("--no-sandbox")
	if cfg.binPath != "" {
		l = l.Bin(cfg.binPath)
	}
	if proxy := os.Getenv("XHS_PROXY"); proxy != "" {
		l = l.Proxy(proxy)
		logrus.Infof("using proxy: %s", maskProxyCredentials(proxy))
	}

	controlURL := l.MustLaunch()
	b := rod.New().ControlURL(controlURL).MustConnect()
	applyCookies(b, cookiesJSON)

	return &Browser{browser: b, launcher: l}
}

func (b *Browser) Close() {
	if b == nil || b.browser == nil {
		return
	}
	if b.isRemote {
		return
	}
	b.browser.MustClose()
	if b.launcher != nil {
		b.launcher.Cleanup()
	}
}

func (b *Browser) NewPage() *rod.Page {
	return stealth.MustPage(b.browser)
}

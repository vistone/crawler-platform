package GoogleEarth

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

type UserAgent struct {
	Version    string
	OS         string
	OSVersion  string
	Language   string
	KMLVersion string
	ClientType string
	AppType    string
}

var (
	versions = []string{
		"7.3.6.10201",
		"7.3.5.10200",
		"7.3.4.10199",
		"7.3.3.10198",
		"7.3.2.10197",
	}

	windowsVersions = map[string]string{
		"Windows 11":  "10.0.22621",
		"Windows 10":  "10.0.19045",
		"Windows 8.1": "6.3.9600",
		"Windows 8":   "6.2.9200.0",
		"Windows 7":   "6.1.7601",
	}

	macOSVersions = []string{
		"10_15_7",
		"11_6_8",
		"12_6_1",
		"13_5_2",
	}

	linuxDistros = []struct {
		Name    string
		Version string
	}{
		{"Ubuntu", "22.04"},
		{"CentOS", "7"},
		{"Debian", "11"},
		{"Fedora", "36"},
	}

	languages = []string{
		"zh-Hans", // 简体中文
		"en-US",   // 英语(美国)
		"ja-JP",   // 日语
		"de-DE",   // 德语
		"fr-FR",   // 法语
		"es-ES",   // 西班牙语
		"ru-RU",   // 俄语
	}

	clientTypes = []string{"Pro", "EC"}
	appTypes    = []string{"default"}
)

func randomWindowsUA() UserAgent {
	var osName string
	var osVersion string
	// 随机选择一个Windows版本
	keys := make([]string, 0, len(windowsVersions))
	for k := range windowsVersions {
		keys = append(keys, k)
	}
	osName = keys[rand.IntN(len(keys))]
	osVersion = windowsVersions[osName]

	return UserAgent{
		Version:    versions[rand.IntN(len(versions))],
		OS:         "Windows",
		OSVersion:  osVersion,
		Language:   languages[rand.IntN(len(languages))],
		KMLVersion: "2.2",
		ClientType: clientTypes[rand.IntN(len(clientTypes))],
		AppType:    appTypes[rand.IntN(len(appTypes))],
	}
}

func randomMacOSUA() UserAgent {
	return UserAgent{
		Version:    versions[rand.IntN(len(versions))],
		OS:         "Macintosh",
		OSVersion:  "Mac OS X " + macOSVersions[rand.IntN(len(macOSVersions))],
		Language:   languages[rand.IntN(len(languages))],
		KMLVersion: "2.2",
		ClientType: clientTypes[rand.IntN(len(clientTypes))],
		AppType:    appTypes[rand.IntN(len(appTypes))],
	}
}

func randomLinuxUA() UserAgent {
	distro := linuxDistros[rand.IntN(len(linuxDistros))]
	return UserAgent{
		Version:    versions[rand.IntN(len(versions))],
		OS:         "Linux",
		OSVersion:  fmt.Sprintf("%s %s", distro.Name, distro.Version),
		Language:   languages[rand.IntN(len(languages))],
		KMLVersion: "2.2",
		ClientType: clientTypes[rand.IntN(len(clientTypes))],
		AppType:    appTypes[rand.IntN(len(appTypes))],
	}
}

func (ua UserAgent) String() string {
	switch ua.OS {
	case "Windows":
		return fmt.Sprintf("GoogleEarth/%s(Windows;Microsoft Windows (%s);%s;kml:%s;client:%s;type:%s)",
			ua.Version, ua.OSVersion, ua.Language, ua.KMLVersion, ua.ClientType, ua.AppType)
	case "Macintosh":
		return fmt.Sprintf("GoogleEarth/%s(Macintosh; Mac OS X %s);%s;kml:%s;client:%s;type:%s)",
			ua.Version, ua.OSVersion, ua.Language, ua.KMLVersion, ua.ClientType, ua.AppType)
	case "Linux":
		return fmt.Sprintf("GoogleEarth/%s(Linux; %s);%s;kml:%s;client:%s;type:%s)",
			ua.Version, ua.OSVersion, ua.Language, ua.KMLVersion, ua.ClientType, ua.AppType)
	default:
		return ""
	}
}

func RandomUserAgent() string {
	// 随机选择操作系统: Windows(60%), Mac(20%), Linux(20%)
	r := rand.IntN(100)
	switch {
	case r < 60:
		return randomWindowsUA().String()
	case r < 80:
		return randomMacOSUA().String()
	default:
		return randomLinuxUA().String()
	}
}

// GetLanguageFromUserAgent 从 Google Earth User-Agent 字符串中提取语言代码
// 例如: "GoogleEarth/7.3.6.10201(Windows;Microsoft Windows (10.0.22621);zh-Hans;..." -> "zh-Hans"
// 例如: "GoogleEarth/7.3.5.10200(Macintosh; Mac OS X 10_15_7);zh-Hans;..." -> "zh-Hans"
func GetLanguageFromUserAgent(ua string) string {
	// Google Earth User-Agent 格式有两种:
	// Windows/Linux: GoogleEarth/版本(OS;OS版本;语言;...)
	// Mac: GoogleEarth/版本(OS; OS版本);语言;...

	// 先尝试找到第一个右括号后的分号，这是 Mac 格式
	closeParenIdx := strings.Index(ua, ")")
	if closeParenIdx > 0 && closeParenIdx < len(ua)-1 {
		// 检查右括号后是否有分号（Mac 格式）
		if closeParenIdx+1 < len(ua) && ua[closeParenIdx+1] == ';' {
			// Mac 格式: 语言在右括号后的第二个分号后
			afterParen := ua[closeParenIdx+2:]
			parts := strings.Split(afterParen, ";")
			if len(parts) > 0 {
				lang := strings.TrimSpace(parts[0])
				if lang != "" {
					return lang
				}
			}
		}
	}

	// Windows/Linux 格式: 在括号内查找第三个分号后的语言代码
	parts := splitUA(ua)
	if len(parts) >= 3 {
		// 第三个部分是语言代码
		return strings.TrimSpace(parts[2])
	}

	// 如果无法解析，返回默认语言
	return "en-US"
}

// splitUA 解析 Google Earth User-Agent 字符串
func splitUA(ua string) []string {
	var parts []string
	var current strings.Builder
	parenCount := 0

	for _, char := range ua {
		if char == '(' {
			parenCount++
			if parenCount == 1 {
				continue
			}
		} else if char == ')' {
			parenCount--
			if parenCount == 0 {
				if current.Len() > 0 {
					parts = append(parts, current.String())
				}
				break
			}
		}

		if parenCount > 0 {
			if char == ';' {
				if current.Len() > 0 {
					parts = append(parts, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(char)
			}
		}
	}

	if current.Len() > 0 && parenCount == 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// ConvertLanguageToAcceptLanguage 将语言代码转换为 Accept-Language 格式
// 例如: "zh-Hans" -> "zh-CN,zh;q=0.9,en;q=0.8"
func ConvertLanguageToAcceptLanguage(langCode string) string {
	// 语言代码到 Accept-Language 的映射
	langMap := map[string]string{
		"zh-Hans": "zh-CN,zh;q=0.9,en;q=0.8",
		"en-US":   "en-US,en;q=0.9",
		"ja-JP":   "ja-JP,ja;q=0.9,en;q=0.8",
		"de-DE":   "de-DE,de;q=0.9,en;q=0.8",
		"fr-FR":   "fr-FR,fr;q=0.9,en;q=0.8",
		"es-ES":   "es-ES,es;q=0.9,en;q=0.8",
		"ru-RU":   "ru-RU,ru;q=0.9,en;q=0.8",
	}

	if acceptLang, ok := langMap[langCode]; ok {
		return acceptLang
	}

	// 默认返回英语
	return "en-US,en;q=0.9"
}

// GetRandomAcceptLanguage 随机选择一个 Accept-Language
func GetRandomAcceptLanguage() string {
	langCode := languages[rand.IntN(len(languages))]
	return ConvertLanguageToAcceptLanguage(langCode)
}

// GetAcceptLanguageFromBrowserUA 根据浏览器 User-Agent 推断合适的 Accept-Language
// 根据平台和地区信息推断可能的语言
func GetAcceptLanguageFromBrowserUA(ua string) string {
	uaLower := strings.ToLower(ua)

	// 根据平台推断可能的语言
	var platformLanguages []string

	// 检测平台
	if strings.Contains(uaLower, "iphone") || strings.Contains(uaLower, "ipad") || strings.Contains(uaLower, "ipod") {
		// iOS 设备 - 常见语言分布
		platformLanguages = []string{"en-US", "zh-Hans", "ja-JP", "de-DE", "fr-FR", "es-ES"}
	} else if strings.Contains(uaLower, "macintosh") || strings.Contains(uaLower, "mac os x") {
		// macOS - 常见语言分布
		platformLanguages = []string{"en-US", "zh-Hans", "ja-JP", "de-DE", "fr-FR", "es-ES", "ru-RU"}
	} else if strings.Contains(uaLower, "windows") {
		// Windows - 常见语言分布（全球使用更广泛）
		platformLanguages = []string{"en-US", "zh-Hans", "ja-JP", "de-DE", "fr-FR", "es-ES", "ru-RU", "en-US"}
	} else if strings.Contains(uaLower, "linux") || strings.Contains(uaLower, "x11") {
		// Linux - 常见语言分布
		platformLanguages = []string{"en-US", "zh-Hans", "de-DE", "fr-FR", "ru-RU", "en-US"}
	} else {
		// 未知平台，使用所有语言
		platformLanguages = languages
	}

	// 从平台常见语言中随机选择一个
	if len(platformLanguages) > 0 {
		langCode := platformLanguages[rand.IntN(len(platformLanguages))]
		return ConvertLanguageToAcceptLanguage(langCode)
	}

	// 默认返回英语
	return "en-US,en;q=0.9"
}

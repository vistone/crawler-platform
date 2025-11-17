package utlsclient_test

import (
	"testing"

	"crawler-platform/utlsclient"
	utls "github.com/refraction-networking/utls"
)

func TestNewLibrary(t *testing.T) {
	lib := utlsclient.NewLibrary()
	if lib == nil {
		t.Fatal("NewLibrary should not return nil")
	}
}

func TestLibraryAll(t *testing.T) {
	lib := utlsclient.NewLibrary()
	profiles := lib.All()

	if len(profiles) == 0 {
		t.Error("Library should contain profiles")
	}

	// 验证每个profile都有必要的字段
	for _, profile := range profiles {
		if profile.Name == "" {
			t.Error("Profile should have a name")
		}
		if profile.UserAgent == "" {
			t.Error("Profile should have a UserAgent")
		}
	}
}

func TestLibraryRandomProfile(t *testing.T) {
	lib := utlsclient.NewLibrary()

	// 多次调用应该返回不同的profile（至少大部分时候）
	profiles := make(map[string]bool)
	for i := 0; i < 10; i++ {
		profile := lib.RandomProfile()
		profiles[profile.Name] = true

		if profile.Name == "" {
			t.Error("RandomProfile should return a profile with a name")
		}
		if profile.UserAgent == "" {
			t.Error("RandomProfile should return a profile with a UserAgent")
		}
	}

	// 应该至少有一些不同的profile
	if len(profiles) < 2 {
		t.Log("Warning: RandomProfile returned mostly the same profiles")
	}
}

func TestLibraryRandomRecommendedProfile(t *testing.T) {
	lib := utlsclient.NewLibrary()

	profile := lib.RandomRecommendedProfile()

	if profile.Name == "" {
		t.Error("RandomRecommendedProfile should return a profile with a name")
	}
	if profile.UserAgent == "" {
		t.Error("RandomRecommendedProfile should return a profile with a UserAgent")
	}
}

func TestLibraryProfileByName(t *testing.T) {
	lib := utlsclient.NewLibrary()

	// 获取所有profile
	allProfiles := lib.All()
	if len(allProfiles) == 0 {
		t.Fatal("Library should have profiles")
	}

	// 测试存在的profile
	testName := allProfiles[0].Name
	profile, err := lib.ProfileByName(testName)
	if err != nil {
		t.Errorf("ProfileByName should find existing profile: %v", err)
	}
	if profile == nil {
		t.Error("ProfileByName should return a profile")
	}
	if profile.Name != testName {
		t.Errorf("ProfileByName returned wrong profile: expected %s, got %s", testName, profile.Name)
	}

	// 测试不存在的profile
	_, err = lib.ProfileByName("NonExistentProfile")
	if err == nil {
		t.Error("ProfileByName should return error for non-existent profile")
	}
}

func TestLibraryProfilesByBrowser(t *testing.T) {
	lib := utlsclient.NewLibrary()

	// 测试Chrome浏览器
	chromeProfiles := lib.ProfilesByBrowser("Chrome")
	if len(chromeProfiles) == 0 {
		t.Error("Should find Chrome profiles")
	}

	for _, profile := range chromeProfiles {
		if profile.Browser != "Chrome" {
			t.Errorf("Profile should be Chrome, got %s", profile.Browser)
		}
	}

	// 测试Firefox浏览器
	firefoxProfiles := lib.ProfilesByBrowser("Firefox")
	if len(firefoxProfiles) == 0 {
		t.Error("Should find Firefox profiles")
	}

	// 测试不存在的浏览器
	unknownProfiles := lib.ProfilesByBrowser("UnknownBrowser")
	if len(unknownProfiles) != 0 {
		t.Error("Should return empty slice for unknown browser")
	}
}

func TestLibraryProfilesByPlatform(t *testing.T) {
	lib := utlsclient.NewLibrary()

	// 测试Windows平台
	windowsProfiles := lib.ProfilesByPlatform("Windows")
	if len(windowsProfiles) == 0 {
		t.Error("Should find Windows profiles")
	}

	for _, profile := range windowsProfiles {
		if profile.Platform != "Windows" {
			t.Errorf("Profile should be Windows, got %s", profile.Platform)
		}
	}

	// 测试不存在的平台
	unknownProfiles := lib.ProfilesByPlatform("UnknownPlatform")
	if len(unknownProfiles) != 0 {
		t.Error("Should return empty slice for unknown platform")
	}
}

func TestLibraryRecommendedProfiles(t *testing.T) {
	lib := utlsclient.NewLibrary()

	recommended := lib.RecommendedProfiles()
	if len(recommended) == 0 {
		t.Error("Should have recommended profiles")
	}

	// 验证推荐profile都是真实浏览器
	for _, profile := range recommended {
		if profile.Browser == "Random" {
			t.Error("Recommended profiles should not include Random")
		}
		if profile.Platform == "Random" {
			t.Error("Recommended profiles should not include Random")
		}
	}
}

func TestLibraryRandomProfileByBrowser(t *testing.T) {
	lib := utlsclient.NewLibrary()

	// 测试存在的浏览器
	profile, err := lib.RandomProfileByBrowser("Chrome")
	if err != nil {
		t.Errorf("RandomProfileByBrowser should find Chrome profile: %v", err)
	}
	if profile == nil {
		t.Error("RandomProfileByBrowser should return a profile")
	}
	if profile.Browser != "Chrome" {
		t.Errorf("Profile should be Chrome, got %s", profile.Browser)
	}

	// 测试不存在的浏览器
	_, err = lib.RandomProfileByBrowser("UnknownBrowser")
	if err == nil {
		t.Error("RandomProfileByBrowser should return error for unknown browser")
	}
}

func TestLibraryRandomProfileByPlatform(t *testing.T) {
	lib := utlsclient.NewLibrary()

	// 测试存在的平台
	profile, err := lib.RandomProfileByPlatform("Windows")
	if err != nil {
		t.Errorf("RandomProfileByPlatform should find Windows profile: %v", err)
	}
	if profile == nil {
		t.Error("RandomProfileByPlatform should return a profile")
	}
	if profile.Platform != "Windows" {
		t.Errorf("Profile should be Windows, got %s", profile.Platform)
	}

	// 测试不存在的平台
	_, err = lib.RandomProfileByPlatform("UnknownPlatform")
	if err == nil {
		t.Error("RandomProfileByPlatform should return error for unknown platform")
	}
}

func TestLibrarySafeProfiles(t *testing.T) {
	lib := utlsclient.NewLibrary()

	safeProfiles := lib.SafeProfiles()
	if len(safeProfiles) == 0 {
		t.Error("Should have safe profiles")
	}

	// 验证安全profile都是Firefox或特定版本
	for _, profile := range safeProfiles {
		isSafe := profile.Browser == "Firefox" ||
			profile.Version == "133" ||
			profile.Version == "131"
		if !isSafe {
			t.Errorf("Profile %s should be marked as safe", profile.Name)
		}
	}
}

func TestLibraryRandomAcceptLanguage(t *testing.T) {
	lib := utlsclient.NewLibrary()

	// 多次调用应该返回不同的语言
	languages := make(map[string]bool)
	for i := 0; i < 10; i++ {
		lang := lib.RandomAcceptLanguage()
		languages[lang] = true

		if lang == "" {
			t.Error("RandomAcceptLanguage should return a language string")
		}
	}

	// 应该至少有一些不同的语言
	if len(languages) < 2 {
		t.Log("Warning: RandomAcceptLanguage returned mostly the same languages")
	}
}

func TestGetRandomFingerprint(t *testing.T) {
	// 测试全局函数
	profile := utlsclient.GetRandomFingerprint()

	if profile.Name == "" {
		t.Error("GetRandomFingerprint should return a profile with a name")
	}
	if profile.UserAgent == "" {
		t.Error("GetRandomFingerprint should return a profile with a UserAgent")
	}
}

func TestProfileStructure(t *testing.T) {
	lib := utlsclient.NewLibrary()
	profiles := lib.All()

	for _, profile := range profiles {
		// 验证HelloID不为空
		if profile.HelloID == (utls.ClientHelloID{}) {
			t.Errorf("Profile %s should have a HelloID", profile.Name)
		}

		// 验证UserAgent格式
		if profile.UserAgent == "" {
			t.Errorf("Profile %s should have a UserAgent", profile.Name)
		}
	}
}


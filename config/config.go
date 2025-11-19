package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
)

// 默认查找路径
const (
	RootConfigPath   = "config.toml"
	FolderConfigPath = "config/config.toml"
)

// GoogleEarthConfig Google Earth 相关配置
type GoogleEarthConfig struct {
	HostName       string `toml:"host_name"`
	TMHostName     string `toml:"tm_host_name"`
	BaseURL        string `toml:"base_url"`
	TMBaseURL      string `toml:"tm_base_url"`
	AuthEndpoint   string `toml:"auth_endpoint"`
	DBRootEndpoint string `toml:"dbroot_endpoint"`
}

// Config 项目配置结构
type Config struct {
	GoogleEarth GoogleEarthConfig `toml:"GoogleEarth"`
}

var (
	loadOnce     sync.Once
	loadErr      error
	globalConfig Config
	configLoaded bool
)

// LoadMergedInto 将项目根目录下的 config.toml 与 config/config.toml 合并后，解码到 out 指针。
// 合并策略：先加载 config/config.toml（作为默认值），再加载根目录 config.toml（作为覆盖）。
// 如果文件不存在则跳过。
func LoadMergedInto(out interface{}) error {
	// 先加载 config/config.toml（默认）
	if fileExists(FolderConfigPath) {
		if _, err := toml.DecodeFile(FolderConfigPath, out); err != nil {
			return fmt.Errorf("解析 %s 失败: %w", FolderConfigPath, err)
		}
	}
	// 根目录 config.toml 覆盖
	if fileExists(RootConfigPath) {
		if _, err := toml.DecodeFile(RootConfigPath, out); err != nil {
			return fmt.Errorf("解析 %s 失败: %w", RootConfigPath, err)
		}
	}
	return nil
}

// MustLoadMergedInto 与 LoadMergedInto 相同，但发生错误时 panic。
func MustLoadMergedInto(out interface{}) {
	loadOnce.Do(func() {
		loadErr = LoadMergedInto(out)
	})
	if loadErr != nil {
		panic(loadErr)
	}
}

// ResolvePath 如果传入相对路径，基于项目根目录返回绝对路径；若已是绝对路径则原样返回。
func ResolvePath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return p, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, p), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

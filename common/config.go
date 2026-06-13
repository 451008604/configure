package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ExtendsField 继承字段名，配置文件中使用此字段声明继承的父配置
const ExtendsField = "__extends"

// MaxExtendsDepth 最大继承深度，防止循环继承导致的无限递归
const MaxExtendsDepth = 10

var (
	configManager *ConfigManager  // 全局配置管理器实例
	configOnce    sync.Once        // 保证配置管理器只初始化一次
)

// ConfigManager 配置管理器
// 支持 JSON 和 YAML 格式的配置文件
// 支持配置继承（通过 __extends 字段）
// 支持嵌套对象的深度合并
// 线程安全，使用 RWMutex 保证并发访问安全
type ConfigManager struct {
	mu       sync.RWMutex
	configs  map[string]map[string]interface{}  // 已解析的配置文件映射
	rawFiles map[string][]byte                   // 原始文件内容，用于 IP 覆盖解析
}

// GetConfigManager 获取配置管理器单例
// 首次调用时会自动加载所有配置文件
func GetConfigManager() *ConfigManager {
	configOnce.Do(func() {
		configManager = &ConfigManager{}
		_ = configManager.Load()
	})
	return configManager
}

// Load 从 conf/json/ 和 conf/yml/ 目录加载所有配置文件
// 解析后的配置存入 configs 映射
// 原始文件内容存入 rawFiles 映射
func (cm *ConfigManager) Load() error {
	newConfigs := make(map[string]map[string]interface{})
	newRawFiles := make(map[string][]byte)

	// 遍历 json 和 yml 两个配置目录
	dirs := []string{"conf/json", "conf/yml"}
	for _, dir := range dirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}
			name := file.Name()
			// 只处理 .json 和 .yml 文件
			if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
				continue
			}

			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			newRawFiles[name] = data

			// 根据文件扩展名选择解析器
			var cfg map[string]interface{}
			if strings.HasSuffix(name, ".json") {
				if err := json.Unmarshal(data, &cfg); err != nil {
					continue
				}
			} else {
				if err := yaml.Unmarshal(data, &cfg); err != nil {
					continue
				}
			}
			newConfigs[name] = cfg
		}
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.configs = newConfigs
	cm.rawFiles = newRawFiles
	return nil
}

// GetConfig 获取指定配置的合并结果
// 如果未指定扩展名，优先尝试 .json，其次 .yml
// 合并逻辑：
// 1. 如果配置包含 __extends 字段，先递归加载父配置
// 2. 父配置为底，子配置覆盖，实现深度合并
// 返回结果以 JSON 格式编码
func (cm *ConfigManager) GetConfig(name string) ([]byte, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if name == "" {
		name = "base.json"
	}

	// 如果未指定扩展名，自动检测
	if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
		if _, ok := cm.configs[name+".json"]; ok {
			name = name + ".json"
		} else if _, ok := cm.configs[name+".yml"]; ok {
			name = name + ".yml"
		} else {
			name = name + ".json"
		}
	}

	merged, err := cm.resolveConfig(name, 0, make(map[string]bool))
	if err != nil {
		return nil, err
	}
	return json.Marshal(merged)
}

// GetMergedConfig 获取指定 IP 的合并配置
// 加载流程：
// 1. 加载 base.json（支持继承链）
// 2. 从 overrides.json 中查找该 IP 的覆盖配置
// 3. 将覆盖配置深度合并到 base 上
// 返回结果以 JSON 格式编码
func (cm *ConfigManager) GetMergedConfig(ip string) ([]byte, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	base, err := cm.resolveConfig("base.json", 0, make(map[string]bool))
	if err != nil {
		return nil, err
	}

	overridesRaw, ok := cm.rawFiles["overrides.json"]
	if ok {
		var overrides map[string]map[string]interface{}
		if err := json.Unmarshal(overridesRaw, &overrides); err == nil {
			if override, ok := overrides[ip]; ok {
				deepMerge(base, override)
			}
		}
	}
	return json.Marshal(base)
}

// resolveConfig 递归解析配置继承链
// 支持 JSON 和 YAML 格式之间的继承
// 安全机制：
// - 最大继承深度限制（MaxExtendsDepth），防止意外递归
// - 循环引用检测（visited 集合），避免无限循环
// 合并逻辑：
// 1. 如果存在 __extends 字段，先递归加载父配置
// 2. 父配置作为底，移除子配置的 __extends 字段后深度合并
// 3. 子配置中的同名字段会覆盖父配置
func (cm *ConfigManager) resolveConfig(name string, depth int, visited map[string]bool) (map[string]interface{}, error) {
	// 检查继承深度，防止无限递归
	if depth > MaxExtendsDepth {
		return nil, fmt.Errorf("extends depth exceeds %d, possible circular reference", MaxExtendsDepth)
	}
	// 检查循环引用
	if visited[name] {
		return nil, fmt.Errorf("circular extends reference detected: %s", name)
	}
	visited[name] = true

	cfg, ok := cm.configs[name]
	if !ok {
		return nil, fmt.Errorf("config file not found: %s", name)
	}

	extends, hasExtends := cfg[ExtendsField]
	merged := deepCopyMap(cfg)

	// 如果存在继承声明，递归加载父配置
	if hasExtends {
		parentName, ok := extends.(string)
		if ok && parentName != "" {
			// 如果父配置未指定扩展名，使用当前文件的扩展名
			if !strings.HasSuffix(parentName, ".json") && !strings.HasSuffix(parentName, ".yml") && !strings.HasSuffix(parentName, ".yaml") {
				if strings.HasSuffix(name, ".json") {
					parentName = parentName + ".json"
				} else {
					parentName = parentName + ".yml"
				}
			}
			parent, err := cm.resolveConfig(parentName, depth+1, visited)
			if err != nil {
				return nil, err
			}
			// 父配置为底，子配置覆盖
			merged = deepCopyMap(parent)
			childCopy := deepCopyMap(cfg)
			delete(childCopy, ExtendsField)
			deepMerge(merged, childCopy)
		}
	}

	return merged, nil
}

// deepCopyMap 深度拷贝 map[string]interface{}
// 递归处理嵌套 map，保证拷贝后的修改不影响原对象
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if vm, ok := v.(map[string]interface{}); ok {
			out[k] = deepCopyMap(vm)
		} else {
			out[k] = v
		}
	}
	return out
}

// deepMerge 深度合并两个 map
// 对于嵌套对象，递归合并而非直接替换
// 例如：
//   dst: {"ServerTCP": {"Address": "127.0.0.1", "Port": "17001"}}
//   src: {"ServerTCP": {"Port": "27001"}}
//   结果: {"ServerTCP": {"Address": "127.0.0.1", "Port": "27001"}}
func deepMerge(dst, src map[string]interface{}) {
	for k, v := range src {
		if vm, ok := v.(map[string]interface{}); ok {
			if dv, ok := dst[k].(map[string]interface{}); ok {
				deepMerge(dv, vm)
				continue
			}
		}
		dst[k] = v
	}
}

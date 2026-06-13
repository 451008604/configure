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

const ExtendsField = "__extends"
const MaxExtendsDepth = 10

var (
	configManager *ConfigManager
	configOnce    sync.Once
)

type ConfigManager struct {
	mu       sync.RWMutex
	configs  map[string]map[string]interface{}
	rawFiles map[string][]byte
}

func GetConfigManager() *ConfigManager {
	configOnce.Do(func() {
		configManager = &ConfigManager{}
		_ = configManager.Load()
	})
	return configManager
}

func (cm *ConfigManager) Load() error {
	newConfigs := make(map[string]map[string]interface{})
	newRawFiles := make(map[string][]byte)

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
			if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
				continue
			}

			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			newRawFiles[name] = data

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

func (cm *ConfigManager) GetConfig(name string) ([]byte, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if name == "" {
		name = "base.json"
	}

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

func (cm *ConfigManager) resolveConfig(name string, depth int, visited map[string]bool) (map[string]interface{}, error) {
	if depth > MaxExtendsDepth {
		return nil, fmt.Errorf("extends depth exceeds %d, possible circular reference", MaxExtendsDepth)
	}
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

	if hasExtends {
		parentName, ok := extends.(string)
		if ok && parentName != "" {
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
			merged = deepCopyMap(parent)
			childCopy := deepCopyMap(cfg)
			delete(childCopy, ExtendsField)
			deepMerge(merged, childCopy)
		}
	}

	return merged, nil
}

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

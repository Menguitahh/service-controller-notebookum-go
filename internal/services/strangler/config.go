package strangler

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Rule struct {
	RoutePattern string `yaml:"route_pattern" json:"route_pattern"`
	Destination  string `yaml:"destination" json:"destination"`
	Enabled      bool   `yaml:"enabled" json:"enabled"`
}

type FileConfig struct {
	Rules []Rule `yaml:"rules" json:"rules"`
}

var (
	cacheMu sync.Mutex
	cache   = map[string]FileConfig{}
)

func DefaultConfigPath() string {
	return filepath.Join("config", "strangler.yaml")
}

func Load(configFile string) (FileConfig, error) {
	path := configFile
	if path == "" {
		if env := os.Getenv("STRANGLER_CONFIG"); env != "" {
			path = env
		} else {
			path = DefaultConfigPath()
		}
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if cfg, ok := cache[path]; ok {
		return cfg, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, err
	}

	var cfg FileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return FileConfig{}, err
	}
	if len(cfg.Rules) == 0 {
		return FileConfig{}, errors.New("invalid YAML in strangler config")
	}

	cache[path] = cfg
	return cfg, nil
}

func FindRule(method, path, configFile string) (*Rule, error) {
	cfg, err := Load(configFile)
	if err != nil {
		return nil, err
	}

	for _, rule := range cfg.Rules {
		if !rule.Enabled {
			continue
		}

		parts := strings.SplitN(rule.RoutePattern, " ", 2)
		if len(parts) != 2 {
			continue
		}

		patternMethod, patternPath := parts[0], parts[1]
		if method != patternMethod {
			continue
		}

		re, err := pathToRegex(patternPath)
		if err != nil {
			return nil, err
		}
		if re.MatchString(path) {
			current := rule
			return &current, nil
		}
	}

	return nil, nil
}

func pathToRegex(patternPath string) (*regexp.Regexp, error) {
	parts := strings.Split(patternPath, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "[^/]+"
		} else {
			parts[i] = regexp.QuoteMeta(part)
		}
	}

	return regexp.Compile("^" + strings.Join(parts, "/") + "$")
}

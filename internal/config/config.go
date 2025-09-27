package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

// Config captures user preferences that affect stacky behaviours.
type Config struct {
	SkipConfirm      bool
	ChangeToMain     bool
	ChangeToAdopted  bool
	ShareSSHSession  bool
	UseMerge         bool
	UseForcePush     bool
	CompactPRDisplay bool
}

// Load reads stacky configuration files from the provided home and repository
// directories. Missing files are ignored and later entries override earlier
// ones, matching the legacy Python behaviour.
func Load(ctx context.Context, homedir, repoRoot string) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		UseForcePush: true, // default matches Python implementation
	}

	paths := make([]string, 0, 2)
	if homedir != "" {
		paths = append(paths, filepath.Join(homedir, ".stackyconfig"))
	}
	if repoRoot != "" {
		paths = append(paths, filepath.Join(repoRoot, ".stackyconfig"))
	}

	for _, path := range paths {
		if ctx.Err() != nil {
			return Config{}, ctx.Err()
		}
		if err := mergeConfigFile(&cfg, path); err != nil {
			return Config{}, err
		}
	}

	return cfg, nil
}

func mergeConfigFile(cfg *Config, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("config: stat %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("config: %s is a directory", path)
	}

	iniFile, err := ini.LoadSources(ini.LoadOptions{IgnoreInlineComment: true, Insensitive: true}, path)
	if err != nil {
		return fmt.Errorf("config: load %s: %w", path, err)
	}

	if section, err := iniFile.GetSection("UI"); err == nil {
		if err := mergeBool(section, "skip_confirm", &cfg.SkipConfirm, path); err != nil {
			return err
		}
		if err := mergeBool(section, "change_to_main", &cfg.ChangeToMain, path); err != nil {
			return err
		}
		if err := mergeBool(section, "change_to_adopted", &cfg.ChangeToAdopted, path); err != nil {
			return err
		}
		if err := mergeBool(section, "share_ssh_session", &cfg.ShareSSHSession, path); err != nil {
			return err
		}
		if err := mergeBool(section, "compact_pr_display", &cfg.CompactPRDisplay, path); err != nil {
			return err
		}
	}

	if section, err := iniFile.GetSection("GIT"); err == nil {
		if err := mergeBool(section, "use_merge", &cfg.UseMerge, path); err != nil {
			return err
		}
		if err := mergeBool(section, "use_force_push", &cfg.UseForcePush, path); err != nil {
			return err
		}
	}

	return nil
}

func mergeBool(section *ini.Section, key string, target *bool, path string) error {
	iniKey := section.Key(key)
	if iniKey == nil || iniKey.Value() == "" {
		return nil
	}
	val, err := iniKey.Bool()
	if err != nil {
		return fmt.Errorf("config: invalid boolean for %s.%s in %s: %w", section.Name(), key, path, err)
	}
	*target = val
	return nil
}

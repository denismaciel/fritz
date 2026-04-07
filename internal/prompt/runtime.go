package prompt

import (
	"os"
	"path/filepath"

	"fritz/internal/config"
	"fritz/internal/skill"
)

type Runtime struct {
	Resources   Resources
	Skills      []skill.Skill
	Diagnostics []skill.Diagnostic
	Profile     Profile
}

func LoadRuntime(cwd string, cfg config.Runtime) (Runtime, error) {
	return LoadRuntimeForProfile(cwd, cfg, ProfileCoding)
}

func LoadRuntimeForProfile(cwd string, cfg config.Runtime, profile Profile) (Runtime, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	resources, err := DiscoverForProfile(DiscoverOptions{
		Cwd:     cwd,
		HomeDir: home,
	}, profile)
	if err != nil {
		return Runtime{}, err
	}
	var roots []string
	if !cfg.Prompt.NoSkills {
		roots = append(roots, resources.SkillRoots...)
	}
	for _, path := range cfg.Prompt.SkillPaths {
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		roots = append(roots, path)
	}
	loaded := skill.Load(skill.LoadOptions{Paths: roots})
	return Runtime{
		Resources:   resources,
		Skills:      loaded.Skills,
		Diagnostics: loaded.Diagnostics,
		Profile:     profile,
	}, nil
}

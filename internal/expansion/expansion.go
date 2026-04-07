package expansion

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var filePattern = regexp.MustCompile(`@([\w\.\-/]+)`)

func ExpandFiles(text string, cwd string) (string, error) {
	var errs []error
	expanded := filePattern.ReplaceAllStringFunc(text, func(match string) string {
		path := match[1:]
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("error reading %s: %v", path, err))
			return match
		}
		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			rel = path
		}
		return fmt.Sprintf("\n\nFile: %s\n```\n%s\n```\n", rel, string(data))
	})
	if len(errs) > 0 {
		return "", errs[0] // Return the first error for simplicity
	}
	return expanded, nil
}

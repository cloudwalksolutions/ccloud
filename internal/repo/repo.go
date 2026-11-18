package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

func App(start, dir string) (string, string, error) {
	for {
		path := filepath.Join(start, dir, "app.yaml")
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			parent := filepath.Dir(start)
			if parent == start {
				return "", "", nil
			}
			start = parent
			continue
		}
		var file struct {
			App string `json:"app"`
		}
		if err == nil {
			err = yaml.Unmarshal(data, &file)
		}
		if err != nil {
			return "", "", fmt.Errorf("cannot read %s: %w", path, err)
		}
		return start, file.App, nil
	}
}

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func checkedRelease(home cloud.Cloud, version string) (string, bool) {
	running, _, _ := strings.Cut(version, " ")
	major, _, _ := strings.Cut(running, ".")
	_, err := strconv.Atoi(major)
	return running, err == nil && home.Releases != "" && os.Getenv("CCLOUD_NO_UPDATE_CHECK") == ""
}

func newerRelease(home cloud.Cloud, running string) string {
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(home.Releases)
	if err != nil {
		return ""
	}
	defer func() { _ = response.Body.Close() }()
	var latest struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	if json.NewDecoder(response.Body).Decode(&latest) != nil {
		return ""
	}
	newest := strings.TrimPrefix(latest.Tag, "v")
	if !older(running, newest) {
		return ""
	}
	return fmt.Sprintf("ccloud %s is out (you have %s): %s\n", newest, running, latest.URL)
}

func dueForUpdateCheck() bool {
	cache, err := os.UserCacheDir()
	if err != nil {
		return false
	}
	stamp := filepath.Join(cache, "ccloud", "update-checked")
	if info, err := os.Stat(stamp); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
		return false
	}
	return os.MkdirAll(filepath.Dir(stamp), 0o700) == nil && os.WriteFile(stamp, nil, 0o600) == nil
}

func older(running, newest string) bool {
	have, want := strings.Split(running, "."), strings.Split(newest, ".")
	for i, part := range want {
		latest, _ := strconv.Atoi(part)
		current := 0
		if i < len(have) {
			current, _ = strconv.Atoi(have[i])
		}
		if current != latest {
			return current < latest
		}
	}
	return false
}

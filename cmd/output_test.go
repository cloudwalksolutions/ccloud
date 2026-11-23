package cmd_test

import (
	"regexp"
	"strings"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var colorCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(text string) string { return colorCodes.ReplaceAllString(text, "") }

func column(line, text string) int {
	GinkgoHelper()
	Expect(line).To(ContainSubstring(text))
	return utf8.RuneCountInString(line[:strings.Index(line, text)])
}

package screenshot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Save(content, label string) (string, error) {
	now := time.Now()
	filename := fmt.Sprintf("loop-screenshot-%s.txt", now.Format("20060102-150405"))
	path := filepath.Join(".", filename)
	cleaned := StripANSI(content)
	if err := os.WriteFile(path, []byte(cleaned), 0644); err != nil {
		return "", err
	}
	return filename, nil
}

func StripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) {
				c := s[i]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
					i++
					break
				}
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

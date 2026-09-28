package settings

import (
	"regexp"
	"strings"
)

var envKey = regexp.MustCompile(`^[\w.-]+$`)

// ParseEnv reads KEY=VALUE lines as dotenv does: "#" comments, optional "export ", values optionally
// in single, double or back quotes (double quotes expand \n and \r), inline "#" comments after
// unquoted values.
func ParseEnv(text string) map[string]string {
	vars := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(rest)
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !envKey.MatchString(key) {
			continue
		}
		vars[key] = parseValue(strings.TrimSpace(value))
	}
	return vars
}

func parseValue(value string) string {
	if value != "" && strings.ContainsRune("'\"`", rune(value[0])) {
		if end := strings.IndexByte(value[1:], value[0]); end >= 0 {
			inner := value[1 : end+1]
			if value[0] == '"' {
				inner = strings.NewReplacer(`\n`, "\n", `\r`, "\r").Replace(inner)
			}
			return inner
		}
	}
	if hash := strings.IndexByte(value, '#'); hash >= 0 {
		value = strings.TrimSpace(value[:hash])
	}
	return value
}

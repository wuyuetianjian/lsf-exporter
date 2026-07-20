package collector

import (
	"strconv"
	"strings"
)

func formatExecutionHosts(value string) string {
	if value == "" {
		return ""
	}

	parts := strings.Split(value, ",")
	counts := make(map[string]int, len(parts))
	ordered := make([]string, 0, len(parts))
	for _, part := range parts {
		host := strings.TrimSpace(part)
		if host == "" {
			continue
		}
		if counts[host] == 0 {
			ordered = append(ordered, host)
		}
		counts[host]++
	}
	if len(ordered) == 0 {
		return ""
	}

	out := make([]string, 0, len(ordered))
	for _, host := range ordered {
		if counts[host] > 1 {
			out = append(out, strconv.Itoa(counts[host])+" * "+host)
			continue
		}
		out = append(out, host)
	}
	return strings.Join(out, ",")
}

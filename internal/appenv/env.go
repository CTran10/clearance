package appenv

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func Must(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		panic(fmt.Sprintf("%s must be set", name))
	}
	return value
}

func String(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func CSV(name string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return fallback
	}
	return items
}

func Int(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func DurationSeconds(name string, fallback time.Duration) time.Duration {
	return time.Duration(Int(name, int(fallback.Seconds()))) * time.Second
}

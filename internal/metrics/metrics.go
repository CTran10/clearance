package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

type Labels map[string]string

type Registry struct {
	mu       sync.Mutex
	counters map[string]counter
}

type counter struct {
	name   string
	labels Labels
	value  int64
}

var Default = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{counters: make(map[string]counter)}
}

func Inc(name string, labels Labels) {
	Default.Inc(name, labels)
}

func Handler() http.Handler {
	return Default.Handler()
}

func (r *Registry) Inc(name string, labels Labels) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := seriesKey(name, labels)
	series := r.counters[key]
	if series.name == "" {
		series.name = name
		series.labels = cloneLabels(labels)
	}
	series.value++
	r.counters[key] = series
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = response.Write([]byte(r.render()))
	})
}

func (r *Registry) render() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	lines := make([]string, 0, len(r.counters))
	for _, series := range r.counters {
		lines = append(lines, formatCounter(series))
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func formatCounter(series counter) string {
	if len(series.labels) == 0 {
		return fmt.Sprintf("%s %d", series.name, series.value)
	}
	keys := sortedLabelKeys(series.labels)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, escape(series.labels[key])))
	}
	return fmt.Sprintf("%s{%s} %d", series.name, strings.Join(parts, ","), series.value)
}

func seriesKey(name string, labels Labels) string {
	keys := sortedLabelKeys(labels)
	parts := []string{name}
	for _, key := range keys {
		parts = append(parts, key, labels[key])
	}
	return strings.Join(parts, "\x00")
}

func sortedLabelKeys(labels Labels) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneLabels(labels Labels) Labels {
	copied := make(Labels, len(labels))
	for key, value := range labels {
		copied[key] = value
	}
	return copied
}

func escape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

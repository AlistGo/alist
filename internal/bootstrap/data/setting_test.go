package data

import (
	"testing"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
)

func TestIsShippedDefault(t *testing.T) {
	const (
		current = "current-default"
		older   = "older-default"
		oldest  = "oldest-default"
	)
	item := &model.SettingItem{Key: "k", Value: current, PreDefault: current}
	legacyDefaults[item.Key] = []string{older, oldest}
	t.Cleanup(func() { delete(legacyDefaults, item.Key) })

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"the default shipped today", current, true},
		{"a default shipped by an earlier version", older, true},
		{"the oldest default we ever shipped", oldest, true},
		{"a value the user chose", "something the user typed", false},
		{"an empty value the user saved", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isShippedDefault(item, tt.value); got != tt.want {
				t.Errorf("isShippedDefault(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

// A setting that never changed its default has no history to carry, so only
// the current default counts as "not customised".
func TestIsShippedDefaultWithoutHistory(t *testing.T) {
	item := &model.SettingItem{Key: "no-history", Value: "only-default", PreDefault: "only-default"}
	if !isShippedDefault(item, "only-default") {
		t.Error("the current default should count as shipped")
	}
	if isShippedDefault(item, "user value") {
		t.Error("a user value must not count as shipped")
	}
}

// The polyfill script AList used to seed into customize_head must be
// recognised in both of the forms it shipped in, so an upgrade drops it
// instead of preserving a third-party script the user never asked for.
// The original polyfill.io domain was sold and served malware in 2024.
func TestCustomizeHeadDropsEveryShippedPolyfill(t *testing.T) {
	if conf.Conf == nil {
		conf.Conf = conf.DefaultConfig()
	}
	var head *model.SettingItem
	for _, item := range InitialSettings() {
		if item.Key == conf.CustomizeHead {
			head = &item
			break
		}
	}
	if head == nil {
		t.Fatal("customize_head is not among the initial settings")
	}

	shipped := []string{
		`<script src="https://polyfill.io/v3/polyfill.min.js?features=String.prototype.replaceAll"></script>`,
		`<script src="https://cdnjs.cloudflare.com/polyfill/v3/polyfill.min.js?features=String.prototype.replaceAll"></script>`,
	}
	for _, value := range shipped {
		if !isShippedDefault(head, value) {
			t.Errorf("customize_head keeps a previously shipped default: %s", value)
		}
	}
	if isShippedDefault(head, `<script src="https://example.com/mine.js"></script>`) {
		t.Error("a script the user added must be preserved")
	}
}

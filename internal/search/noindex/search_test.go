package noindex

import "testing"

func TestMatchKeywords(t *testing.T) {
	cases := []struct {
		name     string
		keywords []string
		want     bool
	}{
		{"Death.End.Cursed.2024.mkv", []string{"death", "cursed"}, true},
		{"Death.End.Cursed.2024.mkv", []string{"Death", "CURSED"}, true}, // case-insensitive
		{"Death.End.Cursed.2024.mkv", []string{"death", "missing"}, false},
		{"holiday photos", nil, true}, // no keywords matches everything
		{"holiday photos", []string{}, true},
		{"a.txt", []string{"a", "txt"}, true},
		{"a.txt", []string{"b"}, false},
	}
	for _, c := range cases {
		if got := matchKeywords(c.name, c.keywords); got != c.want {
			t.Fatalf("matchKeywords(%q, %v) = %v, want %v", c.name, c.keywords, got, c.want)
		}
	}
}

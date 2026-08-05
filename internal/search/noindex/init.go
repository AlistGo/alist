package noindex

import (
	"github.com/alist-org/alist/v3/internal/search/searcher"
)

var config = searcher.Config{
	Name: "no_index",
	// no persisted index, so there is nothing to auto-update
	AutoUpdate: false,
}

func init() {
	searcher.RegisterSearcher(config, func() (searcher.Searcher, error) {
		return &NoIndex{}, nil
	})
}

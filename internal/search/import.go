package search

import (
	_ "github.com/alist-org/alist/v3/internal/search/bleve"
	_ "github.com/alist-org/alist/v3/internal/search/db"
	_ "github.com/alist-org/alist/v3/internal/search/db_non_full_text"
	_ "github.com/alist-org/alist/v3/internal/search/meilisearch"
	_ "github.com/alist-org/alist/v3/internal/search/noindex"
)

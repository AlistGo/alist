package noindex

import (
	"context"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/search/searcher"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/pkg/errors"
)

// maxResults caps how many matches a single live search collects, to bound the
// cost of walking large storages without an index.
const maxResults = 1000

// errStop aborts the walk once enough matches have been collected.
var errStop = errors.New("enough results collected")

// NoIndex is an index-free searcher: instead of querying a prebuilt index it
// walks the filesystem under the requested parent on every search and matches
// object names against the keywords. It trades query speed for not having to
// build and maintain an index. See issue #9468.
type NoIndex struct{}

func (NoIndex) Config() searcher.Config {
	return config
}

func (NoIndex) Search(ctx context.Context, req model.SearchReq) ([]model.SearchNode, int64, error) {
	keywords := strings.Fields(req.Keywords)
	parent := req.Parent
	rootObj, err := fs.Get(ctx, parent, &fs.GetArgs{NoLog: true})
	if err != nil {
		return nil, 0, errors.WithMessagef(err, "failed get dir [%s]", parent)
	}
	maxDepth := setting.GetInt(conf.MaxIndexDepth, 20)
	ignorePaths := conf.SlicesMap[conf.IgnorePaths]

	var nodes []model.SearchNode
	walkFn := func(reqPath string, info model.Obj) error {
		// the parent itself is not a search result
		if reqPath == parent {
			return nil
		}
		for _, ignore := range ignorePaths {
			if strings.HasPrefix(reqPath, ignore) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		// scope: 0 for all, 1 for dir, 2 for file
		if (req.Scope == 1 && !info.IsDir()) || (req.Scope == 2 && info.IsDir()) {
			return nil
		}
		if matchKeywords(info.GetName(), keywords) {
			nodes = append(nodes, model.SearchNode{
				Parent: path.Dir(reqPath),
				Name:   info.GetName(),
				IsDir:  info.IsDir(),
				Size:   info.GetSize(),
			})
			if len(nodes) >= maxResults {
				return errStop
			}
		}
		return nil
	}
	if err := fs.WalkFS(ctx, maxDepth, parent, rootObj, walkFn); err != nil && !errors.Is(err, errStop) {
		return nil, 0, err
	}

	// stable ordering so pagination is consistent across repeated calls
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Name != nodes[j].Name {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].Parent < nodes[j].Parent
	})

	total := int64(len(nodes))
	start := (req.Page - 1) * req.PerPage
	if start >= len(nodes) {
		return []model.SearchNode{}, total, nil
	}
	end := start + req.PerPage
	if end > len(nodes) {
		end = len(nodes)
	}
	return nodes[start:end], total, nil
}

// matchKeywords reports whether name contains every keyword (case-insensitive),
// matching the AND semantics of the indexed searchers. Empty keywords match all.
func matchKeywords(name string, keywords []string) bool {
	lower := strings.ToLower(name)
	for _, kw := range keywords {
		if !strings.Contains(lower, strings.ToLower(kw)) {
			return false
		}
	}
	return true
}

func (NoIndex) Index(ctx context.Context, node model.SearchNode) error {
	return nil
}

func (NoIndex) BatchIndex(ctx context.Context, nodes []model.SearchNode) error {
	return nil
}

func (NoIndex) Get(ctx context.Context, parent string) ([]model.SearchNode, error) {
	return []model.SearchNode{}, nil
}

func (NoIndex) Del(ctx context.Context, prefix string) error {
	return nil
}

func (NoIndex) Release(ctx context.Context) error {
	return nil
}

func (NoIndex) Clear(ctx context.Context) error {
	return nil
}

var _ searcher.Searcher = (*NoIndex)(nil)

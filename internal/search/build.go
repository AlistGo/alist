package search

import (
	"context"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/search/searcher"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/mq"
	"github.com/alist-org/alist/v3/pkg/utils"
	mapset "github.com/deckarep/golang-set/v2"
	log "github.com/sirupsen/logrus"
)

var (
	Quit = atomic.Pointer[chan struct{}]{}
)

func Running() bool {
	return Quit.Load() != nil
}

func BuildIndex(ctx context.Context, indexPaths, ignorePaths []string, maxDepth int, count bool) error {
	var (
		err      error
		objCount uint64 = 0
		fi       model.Obj
	)
	log.Infof("build index for: %+v", indexPaths)
	log.Infof("ignore paths: %+v", ignorePaths)
	quit := make(chan struct{}, 1)
	if !Quit.CompareAndSwap(nil, &quit) {
		// other goroutine is running
		return errs.BuildIndexIsRunning
	}
	var (
		indexMQ = mq.NewInMemoryMQ[ObjWithParent]()
		running = atomic.Bool{} // current goroutine running
		wg      = &sync.WaitGroup{}
	)
	running.Store(true)
	wg.Add(1)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer func() {
			Quit.Store(nil)
			wg.Done()
			// notify walk to exit when StopIndex api called
			running.Store(false)
			ticker.Stop()
		}()
		tickCount := 0
		for {
			select {
			case <-ticker.C:
				tickCount += 1
				if indexMQ.Len() < 1000 && tickCount != 5 {
					continue
				} else if tickCount >= 5 {
					tickCount = 0
				}
				log.Infof("index obj count: %d", objCount)
				indexMQ.ConsumeAll(func(messages []mq.Message[ObjWithParent]) {
					if len(messages) != 0 {
						log.Debugf("current index: %s", messages[len(messages)-1].Content.Parent)
					}
					if err = BatchIndex(ctx, utils.MustSliceConvert(messages,
						func(src mq.Message[ObjWithParent]) ObjWithParent {
							return src.Content
						})); err != nil {
						log.Errorf("build index in batch error: %+v", err)
					} else {
						objCount = objCount + uint64(len(messages))
					}
					if count {
						WriteProgress(&model.IndexProgress{
							ObjCount:     objCount,
							IsDone:       false,
							LastDoneTime: nil,
						})
					}
				})

			case <-quit:
				log.Debugf("build index for %+v received quit", indexPaths)
				eMsg := ""
				now := time.Now()
				originErr := err
				indexMQ.ConsumeAll(func(messages []mq.Message[ObjWithParent]) {
					if err = BatchIndex(ctx, utils.MustSliceConvert(messages,
						func(src mq.Message[ObjWithParent]) ObjWithParent {
							return src.Content
						})); err != nil {
						log.Errorf("build index in batch error: %+v", err)
					} else {
						objCount = objCount + uint64(len(messages))
					}
					if originErr != nil {
						log.Errorf("build index error: %+v", originErr)
						eMsg = originErr.Error()
					} else {
						log.Infof("success build index, count: %d", objCount)
					}
					if count {
						WriteProgress(&model.IndexProgress{
							ObjCount:     objCount,
							IsDone:       true,
							LastDoneTime: &now,
							Error:        eMsg,
						})
					}
				})
				log.Debugf("build index for %+v quit success", indexPaths)
				return
			}
		}
	}()
	defer func() {
		if !running.Load() || Quit.Load() != &quit {
			log.Debugf("build index for %+v stopped by StopIndex", indexPaths)
			return
		}
		select {
		// avoid goroutine leak
		case quit <- struct{}{}:
		default:
		}
		wg.Wait()
	}()
	admin, err := op.GetAdmin()
	if err != nil {
		return err
	}
	if count {
		WriteProgress(&model.IndexProgress{
			ObjCount: 0,
			IsDone:   false,
		})
	}
	for _, indexPath := range indexPaths {
		walkFn := func(indexPath string, info model.Obj) error {
			if !running.Load() {
				return filepath.SkipDir
			}
			for _, avoidPath := range ignorePaths {
				if strings.HasPrefix(indexPath, avoidPath) {
					return filepath.SkipDir
				}
			}
			if storage, _, err := op.GetStorageAndActualPath(indexPath); err == nil {
				if storage.GetStorage().DisableIndex {
					return filepath.SkipDir
				}
			}
			// ignore root
			if indexPath == "/" {
				return nil
			}
			indexMQ.Publish(mq.Message[ObjWithParent]{
				Content: ObjWithParent{
					Obj:    info,
					Parent: path.Dir(indexPath),
				},
			})
			return nil
		}
		fi, err = fs.Get(ctx, indexPath, &fs.GetArgs{})
		if err != nil {
			return err
		}
		// TODO: run walkFS concurrently
		err = fs.WalkFS(context.WithValue(ctx, "user", admin), maxDepth, indexPath, fi, walkFn)
		if err != nil {
			return err
		}
	}
	return nil
}

func Del(ctx context.Context, prefix string) error {
	return instance.Del(ctx, prefix)
}

func Clear(ctx context.Context) error {
	return instance.Clear(ctx)
}

func Config(ctx context.Context) searcher.Config {
	return instance.Config()
}

func Update(parent string, objs []model.Obj) {
	if instance == nil || !instance.Config().AutoUpdate || !setting.GetBool(conf.AutoUpdateIndex) || Running() {
		return
	}
	if isIgnorePath(parent) {
		return
	}
	ctx := context.Background()
	// only update when index have built
	progress, err := Progress()
	if err != nil {
		log.Errorf("update search index error while get progress: %+v", err)
		return
	}
	if !progress.IsDone {
		return
	}
	nodes, err := instance.Get(ctx, parent)
	if err != nil {
		log.Errorf("update search index error while get nodes: %+v", err)
		return
	}
	now := mapset.NewSet[string]()
	for i := range objs {
		now.Add(objs[i].GetName())
	}
	old := mapset.NewSet[string]()
	for i := range nodes {
		old.Add(nodes[i].Name)
	}
	// delete data that no longer exists
	toDelete := old.Difference(now)
	toAdd := now.Difference(old)
	for i := range nodes {
		if toDelete.Contains(nodes[i].Name) && !op.HasStorage(path.Join(parent, nodes[i].Name)) {
			log.Debugf("delete index: %s", path.Join(parent, nodes[i].Name))
			err = instance.Del(ctx, path.Join(parent, nodes[i].Name))
			if err != nil {
				log.Errorf("update search index error while del old node: %+v", err)
				return
			}
		}
	}
	for i := range objs {
		if toAdd.Contains(objs[i].GetName()) {
			if !objs[i].IsDir() {
				log.Debugf("add index: %s", path.Join(parent, objs[i].GetName()))
				err = Index(ctx, parent, objs[i])
				if err != nil {
					log.Errorf("update search index error while index new node: %+v", err)
					return
				}
			} else {
				// build index if it's a folder
				dir := path.Join(parent, objs[i].GetName())
				err = BuildIndex(ctx,
					[]string{dir},
					conf.SlicesMap[conf.IgnorePaths],
					setting.GetInt(conf.MaxIndexDepth, 20)-strings.Count(dir, "/"), false)
				if err != nil {
					log.Errorf("update search index error while build index: %+v", err)
					return
				}
			}
		}
	}
}

func init() {
	op.RegisterObjsUpdateHook(Update)
}

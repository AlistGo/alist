package github_releases

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type GithubReleases struct {
	model.Storage
	Addition

	points []MountPoint
}

func (d *GithubReleases) Config() driver.Config {
	return config
}

func (d *GithubReleases) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *GithubReleases) Init(ctx context.Context) error {
	d.ParseRepos(d.Addition.RepoStructure)
	return nil
}

func (d *GithubReleases) Drop(ctx context.Context) error {
	return nil
}

// processPoint 处理单个挂载点的文件列表
func (d *GithubReleases) processPoint(point *MountPoint, path string, args model.ListArgs) []File {
	var pointFiles []File

	if !d.Addition.ShowAllVersion { // latest
		point.RequestLatestRelease(d.GetRequest, args.Refresh)
		pointFiles = d.processLatestVersion(point, path)
	} else { // all version
		point.RequestReleases(d.GetRequest, args.Refresh)
		pointFiles = d.processAllVersions(point, path)
	}

	return pointFiles
}

// processLatestVersion 处理最新版本的逻辑
func (d *GithubReleases) processLatestVersion(point *MountPoint, path string) []File {
	var pointFiles []File

	if point.Point == path { // 与仓库路径相同
		pointFiles = append(pointFiles, point.GetLatestRelease()...)
		if d.Addition.ShowReadme {
			files := point.GetOtherFile(d.GetRequest, false)
			pointFiles = append(pointFiles, files...)
		}
	} else if strings.HasPrefix(point.Point, path) { // 仓库目录的父目录
		nextDir := GetNextDir(point.Point, path)
		if nextDir != "" {
			dirFile := File{
				Path:     path + "/" + nextDir,
				FileName: nextDir,
				Size:     point.GetLatestSize(),
				UpdateAt: point.Release.PublishedAt,
				CreateAt: point.Release.CreatedAt,
				Type:     "dir",
				Url:      "",
			}
			pointFiles = append(pointFiles, dirFile)
		}
	}

	return pointFiles
}

// processAllVersions 处理所有版本的逻辑
func (d *GithubReleases) processAllVersions(point *MountPoint, path string) []File {
	var pointFiles []File

	if point.Point == path { // 与仓库路径相同
		pointFiles = append(pointFiles, point.GetAllVersion()...)
		if d.Addition.ShowReadme {
			files := point.GetOtherFile(d.GetRequest, false)
			pointFiles = append(pointFiles, files...)
		}
	} else if strings.HasPrefix(point.Point, path) { // 仓库目录的父目录
		nextDir := GetNextDir(point.Point, path)
		if nextDir != "" {
			dirFile := File{
				FileName: nextDir,
				Path:     path + "/" + nextDir,
				Size:     point.GetAllVersionSize(),
				UpdateAt: (*point.Releases)[0].PublishedAt,
				CreateAt: (*point.Releases)[0].CreatedAt,
				Type:     "dir",
				Url:      "",
			}
			pointFiles = append(pointFiles, dirFile)
		}
	} else if strings.HasPrefix(path, point.Point) { // 仓库目录的子目录
		tagName := GetNextDir(path, point.Point)
		if tagName != "" {
			pointFiles = append(pointFiles, point.GetReleaseByTagName(tagName)...)
		}
	}

	return pointFiles
}

// mergeFiles 合并文件列表，处理重复目录
func (d *GithubReleases) mergeFiles(files *[]File, newFiles []File) {
	for _, newFile := range newFiles {
		if newFile.Type == "dir" {
			hasSameDir := false
			for index := range *files {
				if (*files)[index].GetName() == newFile.GetName() && (*files)[index].Type == "dir" {
					hasSameDir = true
					(*files)[index].Size += newFile.Size
					break
				}
			}
			if !hasSameDir {
				*files = append(*files, newFile)
			}
		} else {
			*files = append(*files, newFile)
		}
	}
}

func (d *GithubReleases) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files := make([]File, 0)
	path := fmt.Sprintf("/%s", strings.Trim(dir.GetPath(), "/"))

	if d.Addition.ConcurrentRequests && d.Addition.Token != "" { // 并发处理
		var mu sync.Mutex
		var wg sync.WaitGroup

		for i := range d.points {
			wg.Add(1)
			go func(point *MountPoint) {
				defer wg.Done()
				pointFiles := d.processPoint(point, path, args)

				mu.Lock()
				d.mergeFiles(&files, pointFiles)
				mu.Unlock()
			}(&d.points[i])
		}
		wg.Wait()
	} else { // 串行处理
		for i := range d.points {
			point := &d.points[i]
			pointFiles := d.processPoint(point, path, args)
			d.mergeFiles(&files, pointFiles)
		}
	}

	return utils.SliceConvert(files, func(src File) (model.Obj, error) {
		return src, nil
	})
}

func (d *GithubReleases) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	url := file.GetID()
	gh_proxy := strings.TrimSpace(d.Addition.GitHubProxy)

	if gh_proxy != "" {
		url = strings.Replace(url, "https://github.com", gh_proxy, 1)
	}

	link := model.Link{
		URL:    url,
		Header: http.Header{},
	}
	return &link, nil
}

func (d *GithubReleases) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	// TODO create folder, optional
	return nil, errs.NotImplement
}

func (d *GithubReleases) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	// TODO move obj, optional
	return nil, errs.NotImplement
}

func (d *GithubReleases) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	// TODO rename obj, optional
	return nil, errs.NotImplement
}

func (d *GithubReleases) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	// TODO copy obj, optional
	return nil, errs.NotImplement
}

func (d *GithubReleases) Remove(ctx context.Context, obj model.Obj) error {
	// TODO remove obj, optional
	return errs.NotImplement
}

package github_releases

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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

func (d *GithubReleases) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files := make([]File, 0)
	path := fmt.Sprintf("/%s", strings.Trim(dir.GetPath(), "/"))

	for i := range d.points {
		point := &d.points[i]

		if !d.Addition.ShowAllVersion { // latest
			point.RequestRelease(d.GetRequest, args.Refresh)

			if point.Point == path { // 与仓库路径相同
				files = append(files, point.GetLatestRelease()...)
				if d.Addition.ShowReadme {
					files = append(files, point.GetOtherFile(d.GetRequest, args.Refresh)...)
				}
			} else if strings.HasPrefix(point.Point, path) { // 仓库目录的父目录
				nextDir := GetNextDir(point.Point, path)
				if nextDir == "" {
					continue
				}

				hasSameDir := false
				for index := range files {
					if files[index].GetName() == nextDir {
						hasSameDir = true
						files[index].Size += point.GetLatestSize()
						break
					}
				}
				if !hasSameDir {
					files = append(files, File{
						Path:     path + "/" + nextDir,
						FileName: nextDir,
						Size:     point.GetLatestSize(),
						UpdateAt: point.Release.PublishedAt,
						CreateAt: point.Release.CreatedAt,
						Type:     "dir",
						Url:      "",
					})
				}
			}
		} else { // all version
			point.RequestReleases(d.GetRequest, args.Refresh)

			if point.Point == path { // 与仓库路径相同
				files = append(files, point.GetAllVersion()...)
				if d.Addition.ShowReadme {
					files = append(files, point.GetOtherFile(d.GetRequest, args.Refresh)...)
				}
			} else if strings.HasPrefix(point.Point, path) { // 仓库目录的父目录
				nextDir := GetNextDir(point.Point, path)
				if nextDir == "" {
					continue
				}

				hasSameDir := false
				for index := range files {
					if files[index].GetName() == nextDir {
						hasSameDir = true
						files[index].Size += point.GetAllVersionSize()
						break
					}
				}
				if !hasSameDir {
					files = append(files, File{
						FileName: nextDir,
						Path:     path + "/" + nextDir,
						Size:     point.GetAllVersionSize(),
						UpdateAt: (*point.Releases)[0].PublishedAt,
						CreateAt: (*point.Releases)[0].CreatedAt,
						Type:     "dir",
						Url:      "",
					})
				}
			} else if strings.HasPrefix(path, point.Point) { // 仓库目录的子目录
				tagName := GetNextDir(path, point.Point)
				if tagName == "" {
					continue
				}

				files = append(files, point.GetReleaseByTagName(tagName)...)
			}
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

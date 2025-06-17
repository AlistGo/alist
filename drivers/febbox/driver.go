package febbox

import (
	"context"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

type FebBox struct {
	model.Storage
	Addition
	accessToken string
	oauth2Token oauth2.TokenSource
}

func (d *FebBox) Config() driver.Config {
	return config
}

func (d *FebBox) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *FebBox) Init(ctx context.Context) error {
	// 初始化 oauth2Config
	oauth2Config := &clientcredentials.Config{
		ClientID:     d.ClientID,
		ClientSecret: d.ClientSecret,
		AuthStyle:    oauth2.AuthStyleInParams,
		TokenURL:     "https://api.febbox.com/oauth/token",
	}

	d.initializeOAuth2Token(ctx, oauth2Config, d.Addition.RefreshToken)

	token, err := d.oauth2Token.Token()
	if err != nil {
		return err
	}
	d.accessToken = token.AccessToken
	d.Addition.RefreshToken = token.RefreshToken
	op.MustSaveDriverStorage(d)

	return nil
}

func (d *FebBox) Drop(ctx context.Context) error {
	return nil
}

func (d *FebBox) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.getFilesList(dir.GetID())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src File) (model.Obj, error) {
		return fileToObj(src), nil
	})
}

func (d *FebBox) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var ip string
	if d.Addition.UserIP != "" {
		ip = d.Addition.UserIP
	} else {
		ip = args.IP
	}

	url, err := d.getDownloadLink(file.GetID(), ip)
	if err != nil {
		return nil, err
	}
	return &model.Link{
		URL: url,
	}, nil
}

func (d *FebBox) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	err := d.makeDir(parentDir.GetID(), dirName)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (d *FebBox) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	err := d.move(srcObj.GetID(), dstDir.GetID())
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (d *FebBox) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	err := d.rename(srcObj.GetID(), newName)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (d *FebBox) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	err := d.copy(srcObj.GetID(), dstDir.GetID())
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (d *FebBox) Remove(ctx context.Context, obj model.Obj) error {
	err := d.remove(obj.GetID())
	if err != nil {
		return err
	}

	return nil
}

func (d *FebBox) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	return nil, errs.NotImplement
}

var _ driver.Driver = (*FebBox)(nil)

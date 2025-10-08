package protondrive

/*
Package protondrive
Author: Da3zKi7<da3zki7@duck.com>
Date: 2025-09-18

Thanks to @henrybear327 for modded go-proton-api & Proton-API-Bridge

The power of open-source, the force of teamwork and the magic of reverse engineering!


D@' 3z K!7 - The King Of Cracking

Да здравствует Родина))
*/

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	proton_api_bridge "github.com/henrybear327/Proton-API-Bridge"
	"github.com/henrybear327/Proton-API-Bridge/common"
	"github.com/henrybear327/go-proton-api"
)

type ProtonDrive struct {
	model.Storage
	Addition

	protonDrive *proton_api_bridge.ProtonDrive
	credentials *common.ProtonDriveCredential

	apiBase    string
	appVersion string
	protonJson string
	userAgent  string
	sdkVersion string
	webDriveAV string

	tempServer     *http.Server
	tempServerPort int
	downloadTokens map[string]*downloadInfo
	tokenMutex     sync.RWMutex

	c *proton.Client
	//m *proton.Manager

	credentialCacheFile string

	//userKR   *crypto.KeyRing
	addrKRs  map[string]*crypto.KeyRing
	addrData map[string]proton.Address

	MainShare *proton.Share
	RootLink  *proton.Link

	DefaultAddrKR *crypto.KeyRing
	MainShareKR   *crypto.KeyRing
}

func (d *ProtonDrive) Config() driver.Config {
	return config
}

func (d *ProtonDrive) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *ProtonDrive) Init(ctx context.Context) error {

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("ProtonDrive initialization panic: %v", r)
		}
	}()

	if d.Username == "" {
		return fmt.Errorf("username is required")
	}
	if d.Password == "" {
		return fmt.Errorf("password is required")
	}

	//fmt.Printf("ProtonDrive Init: Username=%s, TwoFACode=%s", d.Username, d.TwoFACode)

	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	cachedCredentials, err := d.loadCachedCredentials()
	useReusableLogin := false
	var reusableCredential *common.ReusableCredentialData

	if err == nil && cachedCredentials != nil &&
		cachedCredentials.UID != "" && cachedCredentials.AccessToken != "" &&
		cachedCredentials.RefreshToken != "" && cachedCredentials.SaltedKeyPass != "" {
		useReusableLogin = true
		reusableCredential = cachedCredentials
	} else {
		useReusableLogin = false
		reusableCredential = &common.ReusableCredentialData{}
	}

	config := &common.Config{
		AppVersion: d.appVersion,
		UserAgent:  d.userAgent,
		FirstLoginCredential: &common.FirstLoginCredentialData{
			Username: d.Username,
			Password: d.Password,
			TwoFA:    d.TwoFACode,
		},
		EnableCaching:              true,
		ConcurrentBlockUploadCount: 5,
		ConcurrentFileCryptoCount:  2,
		UseReusableLogin:           false,
		ReplaceExistingDraft:       true,
		ReusableCredential:         reusableCredential,
		CredentialCacheFile:        d.credentialCacheFile,
	}

	if config.FirstLoginCredential == nil {
		return fmt.Errorf("failed to create login credentials, FirstLoginCredential cannot be nil")
	}

	//fmt.Printf("Calling NewProtonDrive...")

	protonDrive, credentials, err := proton_api_bridge.NewProtonDrive(
		ctx,
		config,
		func(auth proton.Auth) {},
		func() {},
	)

	if credentials == nil && !useReusableLogin {
		return fmt.Errorf("failed to get credentials from NewProtonDrive")
	}

	if err != nil {
		return fmt.Errorf("failed to initialize ProtonDrive: %w", err)
	}

	d.protonDrive = protonDrive

	var finalCredentials *common.ProtonDriveCredential

	if useReusableLogin {

		// For reusable login, create credentials from cached data
		finalCredentials = &common.ProtonDriveCredential{
			UID:           reusableCredential.UID,
			AccessToken:   reusableCredential.AccessToken,
			RefreshToken:  reusableCredential.RefreshToken,
			SaltedKeyPass: reusableCredential.SaltedKeyPass,
		}

		d.credentials = finalCredentials
	} else {
		d.credentials = credentials
	}

	clientOptions := []proton.Option{
		proton.WithAppVersion(d.appVersion),
		proton.WithUserAgent(d.userAgent),
	}
	manager := proton.New(clientOptions...)
	d.c = manager.NewClient(d.credentials.UID, d.credentials.AccessToken, d.credentials.RefreshToken)

	saltedKeyPassBytes, err := base64.StdEncoding.DecodeString(d.credentials.SaltedKeyPass)
	if err != nil {
		return fmt.Errorf("failed to decode salted key pass: %w", err)
	}

	_, addrKRs, addrs, _, err := getAccountKRs(ctx, d.c, nil, saltedKeyPassBytes)
	if err != nil {
		return fmt.Errorf("failed to get account keyrings: %w", err)
	}

	d.MainShare = protonDrive.MainShare
	d.RootLink = protonDrive.RootLink
	d.MainShareKR = protonDrive.MainShareKR
	d.DefaultAddrKR = protonDrive.DefaultAddrKR
	d.addrKRs = addrKRs
	d.addrData = addrs

	return nil
}

func (d *ProtonDrive) Drop(ctx context.Context) error {
	if d.tempServer != nil {
		d.tempServer.Shutdown(ctx)
	}
	return nil
}

func (d *ProtonDrive) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var linkID string

	if dir.GetPath() == "/" {
		linkID = d.protonDrive.RootLink.LinkID
	} else {

		link, err := d.searchByPath(ctx, dir.GetPath(), true)
		if err != nil {
			return nil, err
		}
		linkID = link.LinkID
	}

	entries, err := d.protonDrive.ListDirectory(ctx, linkID)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	//fmt.Printf("Found %d entries for path %s\n", len(entries), dir.GetPath())
	//fmt.Printf("Found %d entries\n", len(entries))

	if len(entries) == 0 {
		emptySlice := []model.Obj{}

		//fmt.Printf("Returning empty slice (entries): %+v\n", emptySlice)

		return emptySlice, nil
	}

	var objects []model.Obj
	for _, entry := range entries {
		obj := &model.Object{
			Name:     entry.Name,
			Size:     entry.Link.Size,
			Modified: time.Unix(entry.Link.ModifyTime, 0),
			IsFolder: entry.IsFolder,
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

func (d *ProtonDrive) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	link, err := d.searchByPath(ctx, file.GetPath(), false)
	if err != nil {
		return nil, err
	}

	if err := d.ensureTempServer(); err != nil {
		return nil, fmt.Errorf("failed to start temp server: %w", err)
	}

	token := d.generateDownloadToken(link.LinkID, file.GetName())

	/* return &model.Link{
		URL: fmt.Sprintf("protondrive://download/%s", link.LinkID),
	}, nil */

	return &model.Link{
		URL: fmt.Sprintf("http://localhost:%d/temp/%s", d.tempServerPort, token),
	}, nil
}

func (d *ProtonDrive) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	var parentLinkID string

	if parentDir.GetPath() == "/" {
		parentLinkID = d.protonDrive.RootLink.LinkID
	} else {
		link, err := d.searchByPath(ctx, parentDir.GetPath(), true)
		if err != nil {
			return nil, err
		}
		parentLinkID = link.LinkID
	}

	_, err := d.protonDrive.CreateNewFolderByID(ctx, parentLinkID, dirName)
	if err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	newDir := &model.Object{
		Name:     dirName,
		IsFolder: true,
		Modified: time.Now(),
	}
	return newDir, nil
}

func (d *ProtonDrive) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return d.DirectMove(ctx, srcObj, dstDir)
}

func (d *ProtonDrive) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {

	if d.protonDrive == nil {
		return nil, fmt.Errorf("protonDrive bridge is nil")
	}

	return d.DirectRename(ctx, srcObj, newName)
}

func (d *ProtonDrive) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if srcObj.IsDir() {
		return nil, fmt.Errorf("directory copy not supported")
	}

	srcLink, err := d.searchByPath(ctx, srcObj.GetPath(), false)
	if err != nil {
		return nil, err
	}

	reader, linkSize, fileSystemAttrs, err := d.protonDrive.DownloadFile(ctx, srcLink, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to download source file: %w", err)
	}
	defer reader.Close()

	actualSize := linkSize
	if fileSystemAttrs != nil && fileSystemAttrs.Size > 0 {
		actualSize = fileSystemAttrs.Size
	}

	tempFile, err := utils.CreateTempFile(reader, actualSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	updatedObj := &model.Object{
		Name: srcObj.GetName(),
		// Use the accurate and real size
		Size:     actualSize,
		Modified: srcObj.ModTime(),
		IsFolder: false,
	}

	return d.Put(ctx, dstDir, &fileStreamer{
		ReadCloser: tempFile,
		obj:        updatedObj,
	}, nil)
}

func (d *ProtonDrive) Remove(ctx context.Context, obj model.Obj) error {
	link, err := d.searchByPath(ctx, obj.GetPath(), obj.IsDir())
	if err != nil {
		return err
	}

	if obj.IsDir() {
		return d.protonDrive.MoveFolderToTrashByID(ctx, link.LinkID, false)
	} else {
		return d.protonDrive.MoveFileToTrashByID(ctx, link.LinkID)
	}
}

func (d *ProtonDrive) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	var parentLinkID string

	if dstDir.GetPath() == "/" {
		parentLinkID = d.protonDrive.RootLink.LinkID
	} else {
		link, err := d.searchByPath(ctx, dstDir.GetPath(), true)
		if err != nil {
			return nil, err
		}
		parentLinkID = link.LinkID
	}

	tempFile, err := utils.CreateTempFile(file, file.GetSize())
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	err = d.uploadFile(ctx, parentLinkID, file.GetName(), tempFile, file.GetSize(), up)
	if err != nil {
		return nil, err
	}

	uploadedObj := &model.Object{
		Name:     file.GetName(),
		Size:     file.GetSize(),
		Modified: file.ModTime(),
		IsFolder: false,
	}
	return uploadedObj, nil
}

func (d *ProtonDrive) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	// TODO get archive file meta-info, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *ProtonDrive) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	// TODO list args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *ProtonDrive) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// TODO return link of file args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *ProtonDrive) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	// TODO extract args.InnerPath path in the archive srcObj to the dstDir location, optional
	// a folder with the same name as the archive file needs to be created to store the extracted results if args.PutIntoNewDir
	// return errs.NotImplement to use an internal archive tool
	return nil, errs.NotImplement
}

var _ driver.Driver = (*ProtonDrive)(nil)

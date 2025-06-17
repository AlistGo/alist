package azure_blob

import (
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
)
// Azure Blob Storage based on the blob APIs
// Link: https://learn.microsoft.com/rest/api/storageservices/blob-service-rest-api
type AzureBlob struct {
	model.Storage
	Addition
	client          *azblob.Client
	containerClient *container.Client
	config          driver.Config
}

// Config returns the driver configuration.
func (d *AzureBlob) Config() driver.Config {
	return d.config
}

// GetAddition returns additional settings specific to Azure Blob Storage.
func (d *AzureBlob) GetAddition() driver.Additional {
	return &d.Addition
}

// Init initializes the Azure Blob Storage client using shared key authentication.
func (d *AzureBlob) Init(ctx context.Context) error {
	// Validate the endpoint URL
	accountName := extractAccountName(d.Addition.Endpoint)
	if !regexp.MustCompile(`^[a-z0-9]+$`).MatchString(accountName) {
		return fmt.Errorf("invalid storage account name: must be chars of lowercase letters or numbers only")
	}

	credential, err := azblob.NewSharedKeyCredential(accountName, d.Addition.AccessKey)
	if err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	// Check if Endpoint is just account name
	endpoint := d.Addition.Endpoint
	if accountName == endpoint {
		endpoint = fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
	}
	// Initialize Azure Blob client with retry policy
	client, err := azblob.NewClientWithSharedKeyCredential(endpoint, credential,
		&azblob.ClientOptions{ClientOptions: azcore.ClientOptions{
			Retry: policy.RetryOptions{
				MaxRetries: MaxRetries,
				RetryDelay: RetryDelay,
			},
		}})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	d.client = client

	// Ensure container exists or create it
	containerName := strings.Trim(d.Addition.ContainerName, "/ \\")
	if containerName == "" {
		return fmt.Errorf("container name cannot be empty")
	}
	return d.createContainerIfNotExists(ctx, containerName)
}

// Drop releases resources associated with the Azure Blob client.
func (d *AzureBlob) Drop(ctx context.Context) error {
	d.client = nil
	return nil
}

// List retrieves blobs and directories under the specified path.
func (d *AzureBlob) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	prefix := ensureTrailingSlash(dir.GetPath())

	pager := d.containerClient.NewListBlobsHierarchyPager("/", &container.ListBlobsHierarchyOptions{
		Prefix: &prefix,
	})

	var objs []model.Obj
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list blobs: %w", err)
		}

		// Process directories
		for _, blobPrefix := range page.Segment.BlobPrefixes {
			objs = append(objs, &model.Object{
				Name:     path.Base(strings.TrimSuffix(*blobPrefix.Name, "/")),
				Path:     *blobPrefix.Name,
				Modified: *blobPrefix.Properties.LastModified,
				Ctime:    *blobPrefix.Properties.CreationTime,
				IsFolder: true,
			})
		}

		// Process files
		for _, blob := range page.Segment.BlobItems {
			if strings.HasSuffix(*blob.Name, "/") {
				continue
			}
			objs = append(objs, &model.Object{
				Name:     path.Base(*blob.Name),
				Path:     *blob.Name,
				Size:     *blob.Properties.ContentLength,
				Modified: *blob.Properties.LastModified,
				Ctime:    *blob.Properties.CreationTime,
				IsFolder: false,
			})
		}
	}
	return objs, nil
}

// Link generates a temporary SAS URL for accessing a blob.
func (d *AzureBlob) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	blobClient := d.containerClient.NewBlobClient(file.GetPath())
	expireDuration := time.Hour * time.Duration(d.SignURLExpire)

	sasURL, err := blobClient.GetSASURL(sas.BlobPermissions{Read: true}, time.Now().Add(expireDuration), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SAS URL: %w", err)
	}
	return &model.Link{URL: sasURL}, nil
}

// MakeDir creates a virtual directory by uploading an empty blob as a marker.
func (d *AzureBlob) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	dirPath := path.Join(parentDir.GetPath(), dirName)
	if err := d.mkDir(ctx, dirPath); err != nil {
		return nil, fmt.Errorf("failed to create directory marker: %w", err)
	}

	return &model.Object{
		Path:     dirPath,
		Name:     dirName,
		IsFolder: true,
	}, nil
}

// Move relocates an object (file or directory) to a new directory.
func (d *AzureBlob) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	srcPath := srcObj.GetPath()
	dstPath := path.Join(dstDir.GetPath(), srcObj.GetName())

	if err := d.moveOrRename(ctx, srcPath, dstPath, srcObj.IsDir(), srcObj.GetSize()); err != nil {
		return nil, fmt.Errorf("move operation failed: %w", err)
	}

	return &model.Object{
		Path:     dstPath,
		Name:     srcObj.GetName(),
		Modified: time.Now(),
		IsFolder: srcObj.IsDir(),
		Size:     srcObj.GetSize(),
	}, nil
}

// Rename changes the name of an existing object.
func (d *AzureBlob) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	srcPath := srcObj.GetPath()
	dstPath := path.Join(path.Dir(srcPath), newName)

	if err := d.moveOrRename(ctx, srcPath, dstPath, srcObj.IsDir(), srcObj.GetSize()); err != nil {
		return nil, fmt.Errorf("rename operation failed: %w", err)
	}

	return &model.Object{
		Path:     dstPath,
		Name:     newName,
		Modified: time.Now(),
		IsFolder: srcObj.IsDir(),
		Size:     srcObj.GetSize(),
	}, nil
}

// Copy duplicates an object (file or directory) to a specified destination directory.
func (d *AzureBlob) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	dstPath := path.Join(dstDir.GetPath(), srcObj.GetName())

	// Handle directory copying using flat listing
	if srcObj.IsDir() {
		srcPrefix := srcObj.GetPath()
		srcPrefix = ensureTrailingSlash(srcPrefix)

		// Get all blobs under the source directory
		blobs, err := d.flattenListBlobs(ctx, srcPrefix)
		if err != nil {
			return nil, fmt.Errorf("failed to list source directory contents: %w", err)
		}

		// Process each blob - copy to destination
		for _, blob := range blobs {
			// Skip the directory marker itself
			if *blob.Name == srcPrefix {
				continue
			}

			// Calculate relative path from source
			relPath := strings.TrimPrefix(*blob.Name, srcPrefix)
			itemDstPath := path.Join(dstPath, relPath)

			if strings.HasSuffix(itemDstPath, "/") || (blob.Metadata["hdi_isfolder"] != nil && *blob.Metadata["hdi_isfolder"] == "true") {
				// Create directory marker at destination
				err := d.mkDir(ctx, itemDstPath)
				if err != nil {
					return nil, fmt.Errorf("failed to create directory marker [%s]: %w", itemDstPath, err)
				}
			} else {
				// Copy the blob
				if err := d.copyFile(ctx, *blob.Name, itemDstPath); err != nil {
					return nil, fmt.Errorf("failed to copy %s: %w", *blob.Name, err)
				}
			}

		}

		// Create directory marker at destination if needed
		if len(blobs) == 0 {
			err := d.mkDir(ctx, dstPath)
			if err != nil {
				return nil, fmt.Errorf("failed to create directory [%s]: %w", dstPath, err)
			}
		}

		return &model.Object{
			Path:     dstPath,
			Name:     srcObj.GetName(),
			Modified: time.Now(),
			IsFolder: true,
		}, nil
	}

	// Copy a single file
	if err := d.copyFile(ctx, srcObj.GetPath(), dstPath); err != nil {
		return nil, fmt.Errorf("failed to copy blob: %w", err)
	}
	return &model.Object{
		Path:     dstPath,
		Name:     srcObj.GetName(),
		Size:     srcObj.GetSize(),
		Modified: time.Now(),
		IsFolder: false,
	}, nil
}

// Remove deletes a specified blob or recursively deletes a directory and its contents.
func (d *AzureBlob) Remove(ctx context.Context, obj model.Obj) error {
	path := obj.GetPath()

	// Handle recursive directory deletion
	if obj.IsDir() {
		return d.deleteFolder(ctx, path)
	}

	// Delete single file
	return d.deleteFile(ctx, path, false)
}

// Put uploads a file stream to Azure Blob Storage with progress tracking.
func (d *AzureBlob) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	blobPath := path.Join(dstDir.GetPath(), stream.GetName())
	blobClient := d.containerClient.NewBlockBlobClient(blobPath)

	// Determine optimal upload options based on file size
	options := optimizedUploadOptions(stream.GetSize())

	// Track upload progress
	progressTracker := &progressTracker{
		total:          stream.GetSize(),
		updateProgress: up,
	}

	// Wrap stream to handle context cancellation and progress tracking
	limitedStream := driver.NewLimitedUploadStream(ctx, io.TeeReader(stream, progressTracker))

	// Upload the stream to Azure Blob Storage
	_, err := blobClient.UploadStream(ctx, limitedStream, options)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	return &model.Object{
		Path:     blobPath,
		Name:     stream.GetName(),
		Size:     stream.GetSize(),
		Modified: time.Now(),
		IsFolder: false,
	}, nil
}

// The following methods related to archive handling are not implemented yet.
// func (d *AzureBlob) GetArchiveMeta(...) {...}
// func (d *AzureBlob) ListArchive(...) {...}
// func (d *AzureBlob) Extract(...) {...}
// func (d *AzureBlob) ArchiveDecompress(...) {...}

// Ensure AzureBlob implements the driver.Driver interface.
var _ driver.Driver = (*AzureBlob)(nil)

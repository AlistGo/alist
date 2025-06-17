package azure_blob

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	Endpoint      string `json:"endpoint" required:"true" default:"https://<accountname>.blob.core.windows.net/" help:"e.g. https://accountname.blob.core.windows.net/. The full endpoint URL for Azure Storage, including the unique storage account name (3 ~ 24 numbers and lowercase letters only)."`
	AccessKey     string `json:"access_key" required:"true" help:"The access key for Azure Storage, used for authentication. https://learn.microsoft.com/azure/storage/common/storage-account-keys-manage"`
	ContainerName string `json:"container_name" required:"true" help:"The name of the container in Azure Storage (created in the Azure portal). https://learn.microsoft.com/azure/storage/blobs/blob-containers-portal"`
	SignURLExpire int    `json:"sign_url_expire" type:"number" default:"4" help:"The expiration time for SAS URLs, in hours."`
}

// implement GetRootId interface
func (r Addition) GetRootId() string {
	return r.ContainerName
}

var config = driver.Config{
	Name:        "Azure Blob Storage",
	LocalSort:   true,
	CheckStatus: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &AzureBlob{
			config: config,
		}
	})
}

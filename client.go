package uploadersdk

import (
	uploader "bytetrade.io/web3os/uploader-sdk/pkg/upload"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
)

func init() {
	_, err := util.GetCommand("restic")
	if err != nil {
		panic(err)
	}
}

type UploadClient struct {
	option uploader.Option
}

// upload
func NewUploadClient(name string,
	userName string,
	password string,
	cloudName string,
	cloudRegion string,
	uploadPath string,
	cloudApiMirror string,
) *UploadClient {
	var opt = uploader.Option{
		Name:           name,
		UserName:       userName,
		Password:       password,
		CloudName:      cloudName,
		CloudRegion:    cloudRegion,
		UploadPath:     uploadPath,
		CloudApiMirror: cloudApiMirror,
	}
	return &UploadClient{
		option: opt,
	}
}

func (c *UploadClient) Upload() error {
	u := &uploader.Upload{}
	return u.Upload(c.option)
}

//  download

type DownloadClient struct {
}

func NewDownloadClient() *DownloadClient {
	return &DownloadClient{}
}

func (c *DownloadClient) Download() error {
	return nil
}

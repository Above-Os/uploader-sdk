package uploadersdk

import (
	"fmt"

	downloader "bytetrade.io/web3os/uploader-sdk/pkg/download"
	uploader "bytetrade.io/web3os/uploader-sdk/pkg/upload"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
	"github.com/pkg/errors"
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

type UploadClientOption struct {
	Name                 string
	UserName             string
	Password             string
	CloudName            string
	CloudRegion          string
	UploadPath           string
	CloudApiMirror       string
	LimitUploadRate      string
	StorageTokenDuration string
}

// upload
func NewUploadClient(opt *UploadClientOption,
) *UploadClient {
	var o = uploader.Option{
		Name:            opt.Name,
		UserName:        opt.UserName,
		Password:        opt.Password,
		CloudName:       opt.CloudName,
		CloudRegion:     opt.CloudRegion,
		UploadPath:      opt.UploadPath,
		CloudApiMirror:  opt.CloudApiMirror,
		LimitUploadRate: opt.LimitUploadRate,
	}
	return &UploadClient{
		option: o,
	}
}

func (c *UploadClient) Upload() error {
	u := &uploader.Upload{}
	return u.Upload(c.option)
}

//  download

type DownloadClient struct {
	option downloader.Option
}

type DownloadClientOption struct {
	Name                 string
	SnapshotId           string
	UserName             string
	Password             string
	CloudName            string
	CloudRegion          string
	DownloadPath         string
	CloudApiMirror       string
	LimitDownloadRate    string
	StorageTokenDuration string
}

func NewDownloadClient(opt *DownloadClientOption) *DownloadClient {
	var o = downloader.Option{
		Name:              opt.Name,
		SnapshotId:        opt.SnapshotId,
		UserName:          opt.UserName,
		Password:          opt.Password,
		CloudName:         opt.CloudName,
		CloudRegion:       opt.CloudRegion,
		DownloadPath:      opt.DownloadPath,
		CloudApiMirror:    opt.CloudApiMirror,
		LimitDownloadRate: opt.LimitDownloadRate,
	}
	return &DownloadClient{
		option: o,
	}
}

func (c *DownloadClient) Download() error {
	if !util.IsExist(c.option.DownloadPath) {
		return errors.WithStack(fmt.Errorf("download path not found"))
	}

	d := &downloader.Download{}

	return d.Download(c.option)
}

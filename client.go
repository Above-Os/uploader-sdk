package uploadersdk

import (
	"fmt"
	"path"
	"path/filepath"

	downloader "bytetrade.io/web3os/uploader-sdk/pkg/download"
	uploader "bytetrade.io/web3os/uploader-sdk/pkg/upload"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
	"bytetrade.io/web3os/uploader-sdk/pkg/util/logger"
	"github.com/pkg/errors"
	"go.uber.org/zap"
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
	BaseDir              string
	Version              string
	Logger               *zap.SugaredLogger
}

// upload
func NewUploadClient(opt *UploadClientOption,
) *UploadClient {
	var o = uploader.Option{
		Name:                 opt.Name,
		UserName:             opt.UserName,
		Password:             opt.Password,
		CloudName:            opt.CloudName,
		CloudRegion:          opt.CloudRegion,
		UploadPath:           opt.UploadPath,
		CloudApiMirror:       opt.CloudApiMirror,
		LimitUploadRate:      opt.LimitUploadRate,
		StorageTokenDuration: opt.StorageTokenDuration,
	}

	var client = &UploadClient{
		option: o,
	}

	client.setLogger(opt.BaseDir, opt.Version, opt.Logger)

	return client
}

func (c *UploadClient) Upload() error {
	u := &uploader.Upload{}
	return u.Upload(c.option)
}

func (c *UploadClient) setLogger(baseDir string, version string, log *zap.SugaredLogger) {
	if log != nil {
		logger.SetLogger(log)
		return
	}

	installerPath := filepath.Join(baseDir, "versions", fmt.Sprintf("v%s", version))
	if err := util.CreateDir(installerPath); err != nil {
		panic(err)
	}

	jsonLogDir := path.Join(baseDir, "logs")
	consoleLogDir := path.Join(installerPath, "logs", "backup_upload.log")
	logger.InitLog(jsonLogDir, consoleLogDir, true)
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
	BaseDir              string
	Version              string
	Logger               *zap.SugaredLogger
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

	var client = &DownloadClient{
		option: o,
	}

	client.setLogger(opt.BaseDir, opt.Version, opt.Logger)

	return client
}

func (c *DownloadClient) Download() error {
	if !util.IsExist(c.option.DownloadPath) {
		return errors.WithStack(fmt.Errorf("download path not found"))
	}

	d := &downloader.Download{}

	return d.Download(c.option)
}

func (c *DownloadClient) setLogger(baseDir string, version string, log *zap.SugaredLogger) {
	if log != nil {
		logger.SetLogger(log)
		return
	}

	installerPath := filepath.Join(baseDir, "versions", fmt.Sprintf("v%s", version))
	if err := util.CreateDir(installerPath); err != nil {
		panic(err)
	}

	jsonLogDir := path.Join(baseDir, "logs")
	consoleLogDir := path.Join(installerPath, "logs", "backup_download.log")
	logger.InitLog(jsonLogDir, consoleLogDir, true)
}

package upload

import (
	"context"

	"bytetrade.io/web3os/uploader-sdk/pkg/restic"
	"bytetrade.io/web3os/uploader-sdk/pkg/storage"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
	"bytetrade.io/web3os/uploader-sdk/pkg/util/logger"
	"github.com/pkg/errors"
)

type UploadProvider interface {
	Upload() error
}

type Upload struct {
	option Option
}

type Option struct {
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

func (u *Upload) Upload(opt Option) error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	u.option = opt

	var storageClient = &storage.StorageClient{
		Name:                 u.option.Name,
		UserName:             u.option.UserName,
		Password:             u.option.Password,
		CloudName:            u.option.CloudName,
		CloudRegion:          u.option.CloudRegion,
		UploadPath:           u.option.UploadPath,
		CloudApiMirror:       u.option.CloudApiMirror,
		LimitUploadRate:      u.option.LimitUploadRate,
		StorageTokenDuration: u.option.StorageTokenDuration,
	}

	var (
		err     error
		exitCh  = make(chan *storage.StorageResponse)
		summary *restic.SummaryOutput
	)

	go storageClient.UploadToStorage(ctx, exitCh)

	select {
	case e, ok := <-exitCh:
		if ok && e.Error != nil {
			err = e.Error
		}
		summary = e.Summary
	case <-ctx.Done():
		err = errors.Errorf("backup %q osdata timed out in 2 hour", u.option.Name)
	}

	if err != nil {
		return err
	}

	if summary != nil {
		logger.Infof("upload successful, data: %s", util.ToJSON(summary))
	}

	return nil
}

package upload

import (
	"context"
	"fmt"

	"bytetrade.io/web3os/uploader-sdk/pkg/restic"
	"bytetrade.io/web3os/uploader-sdk/pkg/storage"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
	"github.com/pkg/errors"
)

type UploadProvider interface {
	Upload() error
}

type Upload struct {
	option Option
}

type Option struct {
	Name           string
	UserName       string
	Password       string
	CloudName      string
	CloudRegion    string
	UploadPath     string
	CloudApiMirror string
}

func (u *Upload) Upload(opt Option) error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	u.option = opt

	var storageClient = &storage.StorageClient{
		Name:           opt.Name,
		UserName:       opt.UserName,
		CloudName:      opt.CloudName,
		CloudRegion:    opt.CloudRegion,
		Password:       opt.Password,
		UploadPath:     opt.UploadPath,
		CloudApiMirror: opt.CloudApiMirror,
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
		err = errors.Errorf("backup %q osdata timed out in 2 hour", opt.Name)
	}

	if err != nil {
		return err
	}

	if summary != nil {
		fmt.Println("backup summary: ", util.PrettyJSON(summary))
	}

	return nil
}

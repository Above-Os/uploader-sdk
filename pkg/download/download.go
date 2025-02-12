package download

import (
	"context"
	"fmt"

	"bytetrade.io/web3os/uploader-sdk/pkg/restic"
	"bytetrade.io/web3os/uploader-sdk/pkg/storage"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
	"github.com/pkg/errors"
)

type DownloadProvider interface {
	Download() error
}

type Download struct {
	option Option
}

type Option struct {
	Name              string
	SnapshotId        string
	UserName          string
	Password          string
	CloudName         string
	CloudRegion       string
	DownloadPath      string
	CloudApiMirror    string
	LimitDownloadRate string
}

func (d *Download) Download(opt Option) error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	d.option = opt
	var storageClient = &storage.StorageClient{
		Name:              d.option.Name,
		SnapshotId:        d.option.SnapshotId,
		UserName:          d.option.UserName,
		Password:          d.option.Password,
		CloudName:         d.option.CloudName,
		CloudRegion:       d.option.CloudRegion,
		DownloadPath:      d.option.DownloadPath,
		CloudApiMirror:    d.option.CloudApiMirror,
		LimitDownloadRate: d.option.LimitDownloadRate,
	}

	var (
		err     error
		exitCh  = make(chan *storage.StorageResponse)
		summary *restic.RestoreSummaryOutput
	)

	go storageClient.Download(ctx, exitCh)

	select {
	case e, ok := <-exitCh:
		if ok && e.Error != nil {
			err = e.Error
		}
		summary = e.RestoreSummary
	case <-ctx.Done():
		err = errors.Errorf("restore %q osdata timed out in 2 hour", d.option.Name)
	}

	if err != nil {
		return err
	}

	if summary != nil {
		fmt.Println("restore summary: ", util.PrettyJSON(summary))
	}

	return nil
}

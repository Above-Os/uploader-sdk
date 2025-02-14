package storage

import (
	"context"

	"bytetrade.io/web3os/backups-sdk/pkg/restic"
	"bytetrade.io/web3os/backups-sdk/pkg/util"
	"bytetrade.io/web3os/backups-sdk/pkg/util/logger"
	"github.com/pkg/errors"
)

type RestoreProvider interface {
	Restore() error
}

type Restore struct {
	option RestoreOption
}

type RestoreOption struct {
	RepoName          string
	SnapshotId        string
	OlaresId          string
	BackupType        string
	Endpoint          string
	AccessKeyId       string
	SecretAccessKey   string
	TargetPath        string
	CloudApiMirror    string
	LimitDownloadRate string
}

func (d *Restore) Restore(opt RestoreOption) error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	d.option = opt
	var storageClient = &StorageClient{
		RepoName:          d.option.RepoName,
		SnapshotId:        d.option.SnapshotId,
		OlaresId:          d.option.OlaresId,
		BackupType:        d.option.BackupType,
		Endpoint:          d.option.Endpoint,
		AccessKeyId:       d.option.AccessKeyId,
		SecretAccessKey:   d.option.SecretAccessKey,
		TargetPath:        d.option.TargetPath,
		CloudApiMirror:    d.option.CloudApiMirror,
		LimitDownloadRate: d.option.LimitDownloadRate,
	}

	var (
		err     error
		exitCh  = make(chan *StorageResponse)
		summary *restic.RestoreSummaryOutput
	)

	go storageClient.RestoreFromStorage(ctx, exitCh)

	select {
	case e, ok := <-exitCh:
		if ok && e.Error != nil {
			err = e.Error
		}
		summary = e.RestoreSummary
	case <-ctx.Done():
		err = errors.Errorf("restore %q osdata timed out in 2 hour", d.option.RepoName)
	}

	if err != nil {
		return err
	}

	if summary != nil {
		logger.Infof("download successful, data: %s", util.ToJSON(summary))
	}

	return nil
}

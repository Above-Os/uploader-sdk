package storage

import (
	"context"

	"bytetrade.io/web3os/backups-sdk/pkg/restic"
	"bytetrade.io/web3os/backups-sdk/pkg/util"
	"bytetrade.io/web3os/backups-sdk/pkg/util/logger"
	"github.com/pkg/errors"
)

type BackupProvider interface {
	Backup() error
}

type Backup struct {
	option BackupOption
}

type BackupOption struct {
	RepoName          string
	OlaresId          string
	BackupType        string
	Endpoint          string
	AccessKeyId       string
	SecretAccessKey   string
	BackupToLocalPath string
	UploadPath        string
	CloudApiMirror    string
	LimitUploadRate   string
}

func (u *Backup) Backup(opt BackupOption) error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	u.option = opt

	var storageClient = &StorageClient{
		RepoName:          u.option.RepoName,
		OlaresId:          u.option.OlaresId,
		BackupType:        u.option.BackupType,
		Endpoint:          u.option.Endpoint,
		AccessKeyId:       u.option.AccessKeyId,
		SecretAccessKey:   u.option.SecretAccessKey,
		BackupToLocalPath: u.option.BackupToLocalPath,
		UploadPath:        u.option.UploadPath,
		CloudApiMirror:    u.option.CloudApiMirror,
		LimitUploadRate:   u.option.LimitUploadRate,
	}

	var (
		err     error
		exitCh  = make(chan *StorageResponse)
		summary *restic.SummaryOutput
	)

	go storageClient.BackupToStorage(ctx, exitCh)

	select {
	case e, ok := <-exitCh:
		if ok && e.Error != nil {
			err = e.Error
		}
		summary = e.Summary
	case <-ctx.Done():
		err = errors.Errorf("backup %q osdata timed out in 2 hour", u.option.RepoName)
	}

	if err != nil {
		return err
	}

	if summary != nil {
		logger.Infof("upload successful, data: %s", util.ToJSON(summary))
	}

	return nil
}

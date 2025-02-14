package storage

import (
	"context"
	"fmt"
	"time"

	"bytetrade.io/web3os/backups-sdk/pkg/restic"
	"bytetrade.io/web3os/backups-sdk/pkg/util"
	"bytetrade.io/web3os/backups-sdk/pkg/util/logger"
)

type StorageClient struct {
	RepoName          string
	SnapshotId        string
	OlaresId          string
	BackupType        string
	Endpoint          string
	AccessKeyId       string
	SecretAccessKey   string
	BackupToLocalPath string

	UploadPath        string
	TargetPath        string
	CloudApiMirror    string
	TokenDuration     string
	LimitUploadRate   string
	LimitDownloadRate string
}

type StorageResponse struct {
	Summary        *restic.SummaryOutput
	RestoreSummary *restic.RestoreSummaryOutput
	Error          error
}

type BackupsOperate string

var (
	OperateBackup  BackupsOperate = "backup"
	OperateRestore BackupsOperate = "restore"
)

func (o BackupsOperate) IsBackup() bool {
	return o == OperateBackup
}

func (s *StorageClient) BackupToStorage(ctx context.Context, exitCh chan<- *StorageResponse) {
	var olaresSpace = &OlaresSpace{
		RepoName:           s.RepoName,
		OlaresId:           s.OlaresId,
		BackupType:         s.BackupType,
		Endpoint:           s.Endpoint,
		AccessKeyId:        s.AccessKeyId,
		SecretAccessKey:    s.SecretAccessKey,
		BackupToLocalPath:  s.BackupToLocalPath,
		Path:               s.UploadPath,
		CloudApiMirror:     s.CloudApiMirror,
		BackupsOperate:     OperateBackup,
		OlaresSpaceSession: new(OlaresSpaceSession),
	}

	if err := olaresSpace.SetAccount(); err != nil {
		exitCh <- &StorageResponse{Error: fmt.Errorf("get account error: %v", err)}
		return
	}
	if err := olaresSpace.EnterPassword(); err != nil {
		exitCh <- &StorageResponse{Error: err}
		return
	}

	var summary *restic.SummaryOutput

	if err := olaresSpace.RefreshToken(false); err != nil {
		exitCh <- &StorageResponse{Error: err}
		return
	}

	for {
		olaresSpace.SetRepoUrl()
		olaresSpace.SetEnv()

		logger.Debugf("get token, data: %s", util.Base64encode([]byte(util.ToJSON(olaresSpace))))

		r, err := restic.NewRestic(ctx, s.RepoName, s.OlaresId, olaresSpace.GetEnv(), &restic.Option{LimitUploadRate: s.LimitUploadRate})
		if err != nil {
			exitCh <- &StorageResponse{Error: err}
			return
		}

		var firstInit = true
		_, err = r.Init()
		if err != nil {
			logger.Debugf("restic init message: %s", err.Error())
			if err.Error() == restic.ERROR_MESSAGE_TOKEN_EXPIRED.Error() {
				logger.Infof("olares space token expired, refresh")
				if err := olaresSpace.RefreshToken(false); err != nil {
					exitCh <- &StorageResponse{Error: fmt.Errorf("get token error: %v", err)}
					return
				}
				time.Sleep(2 * time.Second)
				continue
			} else if err.Error() == restic.ERROR_MESSAGE_ALREADY_INITIALIZED.Error() {
				logger.Infof("restic init skip")
				firstInit = false
			} else {
				exitCh <- &StorageResponse{Error: err}
				return
			}
		}

		if !firstInit {
			logger.Infof("restic repair index, please wait...")
			if err := r.Repair(); err != nil {
				exitCh <- &StorageResponse{Error: err}
				return
			}
		}

		logger.Infof("preparing to start backup, repo: %s", olaresSpace.OlaresSpaceSession.ResticRepo)
		summary, err = r.Backup(s.RepoName, s.UploadPath, "")
		if err != nil {
			switch err.Error() {
			case restic.ERROR_MESSAGE_TOKEN_EXPIRED.Error():
				logger.Infof("olares space token expired, refresh")
				if err := olaresSpace.RefreshToken(false); err != nil {
					exitCh <- &StorageResponse{Error: fmt.Errorf("get token error: %v", err)}
					return
				}
				r.NewContext()
				continue
			default:
				exitCh <- &StorageResponse{Error: err}
				return
			}
		}
		break
	}

	exitCh <- &StorageResponse{Summary: summary}
}

func (s *StorageClient) RestoreFromStorage(ctx context.Context, exitCh chan<- *StorageResponse) {
	var olaresSpace = &OlaresSpace{
		RepoName:           s.RepoName,
		OlaresId:           s.OlaresId,
		BackupType:         s.BackupType,
		Endpoint:           s.Endpoint,
		AccessKeyId:        s.AccessKeyId,
		SecretAccessKey:    s.SecretAccessKey,
		Path:               s.TargetPath,
		CloudApiMirror:     s.CloudApiMirror,
		BackupsOperate:     OperateRestore,
		OlaresSpaceSession: new(OlaresSpaceSession),
	}

	if err := olaresSpace.SetAccount(); err != nil {
		exitCh <- &StorageResponse{Error: fmt.Errorf("get account error: %v", err)}
		return
	}

	if err := olaresSpace.EnterPassword(); err != nil {
		exitCh <- &StorageResponse{Error: err}
		return
	}

	var summary *restic.RestoreSummaryOutput

	if err := olaresSpace.RefreshToken(true); err != nil {
		exitCh <- &StorageResponse{Error: fmt.Errorf("get token error: %v", err)}
		return
	}

	for {
		olaresSpace.SetRepoUrl()
		olaresSpace.SetEnv()

		logger.Debugf("get token, data: %s", util.Base64encode([]byte(util.ToJSON(olaresSpace))))

		r, err := restic.NewRestic(ctx, s.RepoName, s.OlaresId, olaresSpace.GetEnv(), &restic.Option{LimitDownloadRate: s.LimitDownloadRate})
		if err != nil {
			exitCh <- &StorageResponse{Error: err}
			return
		}

		snapshotSummary, err := r.GetSnapshot(s.SnapshotId)
		if err != nil {
			exitCh <- &StorageResponse{Error: err}
			return
		}
		var uploadPath = snapshotSummary.Paths[0]

		logger.Infof("snapshot %s detail: %s", s.SnapshotId, util.ToJSON(snapshotSummary))

		summary, err = r.Restore(s.SnapshotId, uploadPath, s.TargetPath)
		if err != nil {
			switch err.Error() {
			case restic.ERROR_MESSAGE_TOKEN_EXPIRED.Error():
				logger.Infof("olares space token expired, refresh")
				if err := olaresSpace.RefreshToken(false); err != nil {
					exitCh <- &StorageResponse{Error: fmt.Errorf("get token error: %v", err)}
					return
				}
				r.NewContext()
				time.Sleep(2 * time.Second)
				continue
			default:
				exitCh <- &StorageResponse{Error: err}
				return
			}
		}
		break
	}

	exitCh <- &StorageResponse{RestoreSummary: summary}
}

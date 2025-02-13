package storage

import (
	"context"
	"fmt"

	"bytetrade.io/web3os/uploader-sdk/pkg/restic"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
	"bytetrade.io/web3os/uploader-sdk/pkg/util/logger"
)

type StorageClient struct {
	Name                 string
	SnapshotId           string
	UserName             string
	CloudName            string
	CloudRegion          string
	Password             string
	UploadPath           string
	DownloadPath         string
	CloudApiMirror       string
	TokenDuration        string
	LimitUploadRate      string
	LimitDownloadRate    string
	StorageTokenDuration string
}

type StorageResponse struct {
	Summary        *restic.SummaryOutput
	RestoreSummary *restic.RestoreSummaryOutput
	Error          error
}

func (s *StorageClient) UploadToStorage(ctx context.Context, exitCh chan<- *StorageResponse) {
	var olaresSpace = &OlaresSpace{
		UserName:       s.UserName,
		CloudName:      s.CloudName,
		CloudRegion:    s.CloudRegion,
		UploadPath:     s.UploadPath,
		CloudApiMirror: s.CloudApiMirror,
		Duration:       s.StorageTokenDuration,
	}

	if err := olaresSpace.SetAccount(); err != nil {
		exitCh <- &StorageResponse{Error: fmt.Errorf("get account error: %v", err)}
		return
	}

	var summary *restic.SummaryOutput

	if err := olaresSpace.RefreshToken(true); err != nil {
		exitCh <- &StorageResponse{Error: err}
		return
	}

	for {
		olaresSpace.SetRepoUrl(s.Name, s.Password)
		olaresSpace.SetEnv()

		logger.Infof("get token, data: %s", util.ToJSON(olaresSpace))

		r, err := restic.NewRestic(ctx, s.Name, s.UserName, olaresSpace.GetEnv(), &restic.Option{LimitUploadRate: s.LimitUploadRate})
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

		summary, err = r.Backup(s.Name, s.UploadPath, "")
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

func (s *StorageClient) Download(ctx context.Context, exitCh chan<- *StorageResponse) {
	var olaresSpace = &OlaresSpace{
		UserName:       s.UserName,
		CloudName:      s.CloudName,
		CloudRegion:    s.CloudRegion,
		CloudApiMirror: s.CloudApiMirror,
		Duration:       s.StorageTokenDuration,
	}

	if err := olaresSpace.SetAccount(); err != nil {
		exitCh <- &StorageResponse{Error: fmt.Errorf("get account error: %v", err)}
		return
	}

	var summary *restic.RestoreSummaryOutput

	if err := olaresSpace.RefreshToken(true); err != nil {
		exitCh <- &StorageResponse{Error: fmt.Errorf("get token error: %v", err)}
		return
	}

	for {
		olaresSpace.SetRepoUrl(s.Name, s.Password)
		olaresSpace.SetEnv()

		logger.Infof("get token, data: %s", util.ToJSON(olaresSpace))

		r, err := restic.NewRestic(ctx, s.Name, s.UserName, olaresSpace.GetEnv(), &restic.Option{LimitDownloadRate: s.LimitDownloadRate})
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

		summary, err = r.Restore(s.SnapshotId, uploadPath, s.DownloadPath)
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

	exitCh <- &StorageResponse{RestoreSummary: summary}
}

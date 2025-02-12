package storage

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"bytetrade.io/web3os/uploader-sdk/pkg/common"
	"bytetrade.io/web3os/uploader-sdk/pkg/restic"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
)

type StorageClient struct {
	Name              string
	SnapshotId        string
	UserName          string
	CloudName         string
	CloudRegion       string
	Password          string
	UploadPath        string
	DownloadPath      string
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

func (s *StorageClient) UploadToStorage(ctx context.Context, exitCh chan<- *StorageResponse) {
	var olaresSpace = &OlaresSpace{
		UserName:       s.UserName,
		CloudName:      s.parseCloudName(),
		CloudRegion:    s.CloudRegion,
		UploadPath:     s.UploadPath,
		CloudApiMirror: s.CloudApiMirror,
		Duration:       s.parseDuration(),
	}

	if err := olaresSpace.SetAccount(); err != nil {
		exitCh <- &StorageResponse{Error: fmt.Errorf("get account error: %v", err)}
		return
	}

	var summary *restic.SummaryOutput

	if err := olaresSpace.RefreshToken(true); err != nil {
		exitCh <- &StorageResponse{Error: fmt.Errorf("get token error: %v", err)}
		return
	}

	fmt.Println(util.PrettyJSON(olaresSpace))

	for {
		olaresSpace.SetRepoUrl(s.Name, s.Password)
		olaresSpace.SetEnv()

		r, err := restic.NewRestic(ctx, olaresSpace.GetEnv(), &restic.Option{LimitUploadRate: s.LimitUploadRate})
		if err != nil {
			exitCh <- &StorageResponse{Error: err}
			return
		}

		var firstInit = true
		_, err = r.Init()
		if err != nil {
			if err.Error() == restic.ERROR_MESSAGE_TOKEN_EXPIRED.Error() {
				fmt.Println("refresh expired token")
				if err := olaresSpace.RefreshToken(false); err != nil {
					exitCh <- &StorageResponse{Error: fmt.Errorf("get token error: %v", err)}
					return
				}
				continue
			} else if err.Error() == restic.ERROR_MESSAGE_ALREADY_INITIALIZED.Error() {
				firstInit = false
			} else {
				exitCh <- &StorageResponse{Error: err}
				return
			}
		}

		if !firstInit {
			fmt.Println("repair index")
			if err := r.Repair(); err != nil {
				exitCh <- &StorageResponse{Error: err}
				return
			}
		}

		summary, err = r.Backup(s.Name, s.UploadPath, "")
		if err != nil {
			switch err.Error() {
			case restic.ERROR_MESSAGE_TOKEN_EXPIRED.Error():
				fmt.Println("refresh expired token")
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
		CloudName:      s.parseCloudName(),
		CloudRegion:    s.CloudRegion,
		CloudApiMirror: s.CloudApiMirror,
		Duration:       s.parseDuration(),
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

	fmt.Println(util.PrettyJSON(olaresSpace))

	for {
		olaresSpace.SetRepoUrl(s.Name, s.Password)
		olaresSpace.SetEnv()

		r, err := restic.NewRestic(ctx, olaresSpace.GetEnv(), &restic.Option{LimitDownloadRate: s.LimitDownloadRate})
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

		fmt.Printf("snapshot %s detail %v\n", s.SnapshotId, util.PrettyJSON(snapshotSummary))

		summary, err = r.Restore(s.SnapshotId, uploadPath, s.DownloadPath)
		if err != nil {
			switch err.Error() {
			case restic.ERROR_MESSAGE_TOKEN_EXPIRED.Error():
				fmt.Println("refresh token expired")
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

func (s *StorageClient) parseDuration() time.Duration {
	var defaultDuration = 12 * time.Hour
	if s.TokenDuration == "" {
		return defaultDuration
	}

	res, err := strconv.ParseInt(s.TokenDuration, 10, 64)
	if err != nil {
		return defaultDuration
	}
	dur, err := time.ParseDuration(fmt.Sprintf("%dm", res))
	if err != nil {
		return defaultDuration
	}

	return dur
}

func (s *StorageClient) parseCloudName() string {
	switch s.CloudName {
	case common.TencentCloudName:
		return common.TencentCloudName
	case common.AliCloudName:
		return common.AliCloudName
	default:
		return common.AWSCloudName
	}
}

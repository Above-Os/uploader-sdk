package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bytetrade.io/web3os/uploader-sdk/pkg/restic"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
)

type StorageClient struct {
	Name           string
	SnapshotId     string
	UserName       string
	CloudName      string
	CloudRegion    string
	Password       string
	UploadPath     string
	DownloadPath   string
	CloudApiMirror string
}

type StorageResponse struct {
	Summary *restic.SummaryOutput
	Error   error
}

func (s *StorageClient) UploadToStorage(ctx context.Context, exitCh chan<- *StorageResponse) {
	var olaresSpace = &OlaresSpace{
		UserName:       s.UserName,
		CloudName:      s.CloudName,
		CloudRegion:    s.CloudRegion,
		UploadPath:     s.UploadPath,
		CloudApiMirror: s.CloudApiMirror,
		Duration:       15 * time.Minute,
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

		r, err := restic.NewRestic(ctx, olaresSpace.GetEnv())
		if err != nil {
			exitCh <- &StorageResponse{Error: err}
			return
		}

		var firstInit = true
		res, err := r.Init()
		if err != nil {
			exitCh <- &StorageResponse{Error: err}
			return
		}
		if strings.Contains(res, restic.ERROR_MESSAGE_ALREADY_INITIALIZED.Error()) {
			firstInit = false
		} else if strings.Contains(res, restic.ERROR_MESSAGE_UNABLE_TO_OPEN_REPOSITORY.Error()) && strings.Contains(res, restic.ERROR_MESSAGE_BAD_REQUEST.Error()) {
			fmt.Println("refresh expired token")
			if err := olaresSpace.RefreshToken(false); err != nil {
				exitCh <- &StorageResponse{Error: fmt.Errorf("get token error: %v", err)}
				return
			}
			continue
		} else {
			exitCh <- &StorageResponse{Error: err}
			return
		}

		if !firstInit {
			fmt.Println("repair index")
			if err := r.Repair(); err != nil {
				exitCh <- &StorageResponse{Error: err}
				return
			}
		}

		summary, err = r.Backup(s.UploadPath, "")
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

package restic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"bytetrade.io/web3os/uploader-sdk/pkg/util"
	"bytetrade.io/web3os/uploader-sdk/pkg/util/cmd"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

const (
	LIMIT_DOWNLOAD = "10"
	LIMIT_UPLOAD   = "300" // KB
)

type RESTIC_ERROR_MESSAGE string

const (
	SUCCESS_MESSAGE_REPAIR_INDEX             RESTIC_ERROR_MESSAGE = "adding pack file to index"
	ERROR_MESSAGE_UNABLE_TO_OPEN_REPOSITORY  RESTIC_ERROR_MESSAGE = "unable to open repository"
	ERROR_MESSAGE_BAD_REQUEST                RESTIC_ERROR_MESSAGE = "400 Bad Request"
	ERROR_MESSAGE_TOKEN_EXPIRED              RESTIC_ERROR_MESSAGE = "The provided token has expired"
	ERROR_MESSAGE_UNABLE_TO_OPEN_CONFIG_FILE RESTIC_ERROR_MESSAGE = "unable to open config file: Stat: 400 Bad Request"
	ERROR_MESSAGE_CONFIG_INVALID             RESTIC_ERROR_MESSAGE = "config invalid, please chek repository or authorization config"
	ERROR_MESSAGE_LOCKED                     RESTIC_ERROR_MESSAGE = "repository is already locked by"
	ERROR_MESSAGE_ALREADY_INITIALIZED        RESTIC_ERROR_MESSAGE = "repository master key and config already initialized"
)

const (
	PARAM_INSECURE_TLS = "--insecure-tls"
)

func (e RESTIC_ERROR_MESSAGE) Error() string {
	return string(e)
}

const (
	resticFile = "restic"
	tolerance  = 1e-9

	PRINT_START_MESSAGE    = "[Upload] start, files: %d, size: %s\n"
	PRINT_PROGRESS_MESSAGE = "[Upload] progress %s, files: %d/%d, size: %s/%s, current: %v\n"
	PRINT_FINISH_MESSAGE   = "[Upload] finished, files: %d, size: %s, please waiting...\n"
	PRINT_SUCCESS_MESSAGE  = ""
)

type Restic interface {
	Init() (string, error)
	Backup(folder string, filePathPrefix string) (*SummaryOutput, error)
	Repair() error
	Unlock() (string, error)
	NewContext()
	RefreshEnv(envs map[string]string)
	Cancel()
}

type resticManager struct {
	ctx    context.Context
	cancel context.CancelFunc
	envs   map[string]string
	bin    string
}

func NewRestic(ctx context.Context, envs map[string]string) (Restic, error) {
	var commandPath, err = util.GetCommand(resticFile)
	if err != nil {
		return nil, err
	}
	var ctxRestic, cancel = context.WithCancel(ctx)
	return &resticManager{
		ctx:    ctxRestic,
		cancel: cancel,
		envs:   envs,
		bin:    commandPath,
	}, nil
}

func (r *resticManager) NewContext() {
	r.ctx, r.cancel = context.WithCancel(r.ctx)
}

func (r *resticManager) Cancel() {
	r.cancel()
}

func (r *resticManager) RefreshEnv(envs map[string]string) {
	r.envs = envs
}

func (r *resticManager) Init() (string, error) {
	opts := cmd.CommandOptions{
		Path: r.bin,
		Args: []string{
			"init",
			"--json",
			PARAM_INSECURE_TLS,
		},
		Envs: r.envs,
	}
	c := cmd.NewCommand(r.ctx, opts)
	sb := new(strings.Builder)

	go func() {
		for {
			select {
			case res, ok := <-c.Ch:
				if !ok {
					return
				}
				if res == nil || len(res) == 0 {
					continue
				}
				sb.WriteString(string(res) + "\n")
			case <-r.ctx.Done():
				return
			}
		}
	}()

	_, err := c.Run()
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}

func (r *resticManager) Backup(folder string, filePathPrefix string) (*SummaryOutput, error) {
	var backupCtx, cancel = context.WithCancel(r.ctx)
	defer cancel()
	opts := cmd.CommandOptions{
		Path: r.bin,
		Args: []string{
			"backup",
			"--limit-upload",
			LIMIT_UPLOAD,
			"--json",
			folder,
			PARAM_INSECURE_TLS,
		},
		Envs: r.envs,
	}
	c := cmd.NewCommand(backupCtx, opts)

	var prevPercent float64
	var finished bool
	var summary *SummaryOutput
	var errorMsg RESTIC_ERROR_MESSAGE

	go func() {
		for {
			select {
			case res, ok := <-c.Ch:
				if !ok {
					return
				}
				if res == nil || len(res) == 0 {
					continue
				}

				status := messagePool.Get()
				if err := json.Unmarshal(res, status); err != nil {
					messagePool.Put(status)
					var msg = string(res)

					switch {
					case strings.Contains(msg, ERROR_MESSAGE_TOKEN_EXPIRED.Error()):
						errorMsg = ERROR_MESSAGE_TOKEN_EXPIRED
						c.Cancel()
						return
					case strings.Contains(msg, ERROR_MESSAGE_UNABLE_TO_OPEN_CONFIG_FILE.Error()):
						errorMsg = ERROR_MESSAGE_UNABLE_TO_OPEN_CONFIG_FILE
						c.Cancel()
						return
					default:
						errorMsg = RESTIC_ERROR_MESSAGE(msg)
						c.Cancel()
						return
					}
				}
				switch status.MessageType {
				case "status":
					switch {
					case math.Abs(status.PercentDone-0.0) < tolerance:
						fmt.Printf(PRINT_START_MESSAGE, status.TotalFiles, util.FormatBytes(status.TotalBytes))
					case math.Abs(status.PercentDone-1.0) < tolerance:
						if !finished {
							fmt.Printf(PRINT_FINISH_MESSAGE, status.TotalFiles, util.FormatBytes(status.TotalBytes))
							finished = true
						}
					default:
						if prevPercent != status.PercentDone {
							fmt.Printf(PRINT_PROGRESS_MESSAGE,
								status.GetPercentDone(),
								status.FilesDone,
								status.TotalFiles,
								util.FormatBytes(status.BytesDone),
								util.FormatBytes(status.TotalBytes),
								r.fileNameTidy(status.CurrentFiles, filePathPrefix),
							)
						}
						prevPercent = status.PercentDone
					}
				case "summary":
					if err := json.Unmarshal(res, &summary); err != nil {
						messagePool.Put(status)
						return
					}
					fmt.Printf("all succeed, blobs: %d, snapshot: %s\n",
						summary.DataBlobs,
						summary.SnapshotID,
					)
					messagePool.Put(status)
					return
				}
				messagePool.Put(status)
			case <-r.ctx.Done():
				return
			}
		}
	}()

	_, err := c.Run()
	if err != nil {
		return nil, err
	}
	if errorMsg != "" {
		return nil, fmt.Errorf(errorMsg.Error())
	}
	return summary, nil
}

func (r *resticManager) Repair() error {
	backoff := wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    10,
	}

	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		res, err := r.repairIndex()
		if err != nil {
			return err
		}
		fmt.Println(res)
		if strings.Contains(res, ERROR_MESSAGE_LOCKED.Error()) {
			ures, _ := r.Unlock()
			fmt.Println(ures)
			return fmt.Errorf("retry")
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (r *resticManager) repairIndex() (string, error) {
	opts := cmd.CommandOptions{
		Path:  r.bin,
		Args:  []string{"repair", "index", PARAM_INSECURE_TLS},
		Envs:  r.envs,
		Print: true,
	}
	c := cmd.NewCommand(r.ctx, opts)

	sb := new(strings.Builder)
	go func() {
		for {
			select {
			case res, ok := <-c.Ch:
				if !ok {
					return
				}
				if res == nil || len(res) == 0 {
					continue
				}
				sb.WriteString(string(res) + "\n")
			case <-r.ctx.Done():
				return
			}
		}
	}()

	_, err := c.Run()
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}

func (r *resticManager) Unlock() (string, error) {
	opts := cmd.CommandOptions{
		Path: r.bin,
		Args: []string{"unlock", "--remove-all", PARAM_INSECURE_TLS},
		Envs: r.envs,
	}
	c := cmd.NewCommand(r.ctx, opts)
	sb := new(strings.Builder)

	go func() {
		for {
			select {
			case res, ok := <-c.Ch:
				if !ok {
					return
				}
				if res == nil || len(res) == 0 {
					continue
				}
				sb.WriteString(string(res) + "\n")
			case <-r.ctx.Done():
				return
			}
		}
	}()

	_, err := c.Run()
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}

func (r *resticManager) fileNameTidy(f []string, prefix string) []string {
	if f == nil || len(f) == 0 {
		return f
	}

	var res []string
	for _, file := range f {
		res = append(res, strings.TrimPrefix(file, prefix))
	}

	return res
}

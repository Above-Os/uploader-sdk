package restic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"bytetrade.io/web3os/uploader-sdk/pkg/util"
	"bytetrade.io/web3os/uploader-sdk/pkg/util/cmd"
	"bytetrade.io/web3os/uploader-sdk/pkg/util/logger"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
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
	ERROR_MESSAGE_SNAPSHOT_NOT_FOUND         RESTIC_ERROR_MESSAGE = "failed to find snapshot: no matching ID found for prefix"
)

const (
	PARAM_JSON_OUTPUT  = "--json"
	PARAM_INSECURE_TLS = "--insecure-tls"
)

func (e RESTIC_ERROR_MESSAGE) Error() string {
	return string(e)
}

const (
	resticFile = "restic"
	tolerance  = 1e-9

	PRINT_START_MESSAGE    = "[Upload] start, files: %d, size: %s"
	PRINT_PROGRESS_MESSAGE = "[Upload] progress %s, files: %d/%d, size: %s/%s, current: %v"
	PRINT_FINISH_MESSAGE   = "[Upload] finished, files: %d, size: %s, please waiting..."

	PRINT_RESTORE_START_MESSAGE    = "[Download] start, files: %d, size: %s"
	PRINT_RESTORE_PROGRESS_MESSAGE = "[Download] progress %s, files: %d/%d, size: %s/%s"
	PRINT_RESTORE_ITEM             = "[Download] restored file: %s, size: %s"
	PRINT_RESTORE_FINISH_MESSAGE   = "[Download] snapshot %s finished, total files: %d, restored files: %d, total size: %s, restored size: %s, please waiting..."
	PRINT_SUCCESS_MESSAGE          = ""
)

type Restic interface {
	Init() (*InitSummaryOutput, error)
	Backup(name string, folder string, filePathPrefix string) (*SummaryOutput, error)
	Repair() error
	Unlock() (string, error)
	Restore(snapshotId string, uploadPath string, target string) (*RestoreSummaryOutput, error)
	NewContext()
	RefreshEnv(envs map[string]string)
	GetSnapshot(snapshotId string) (*Snapshot, error)
	Cancel()
}

type resticManager struct {
	ctx    context.Context
	cancel context.CancelFunc
	name   string
	user   string
	envs   map[string]string
	bin    string
	opt    *Option
}

type Option struct {
	LimitDownloadRate string
	LimitUploadRate   string
}

func (o *Option) uploadRate() string {
	var defaultUploadRate = "--limit-upload=0"
	if o.LimitUploadRate == "" {
		return defaultUploadRate
	}

	res, err := strconv.ParseInt(o.LimitUploadRate, 10, 64)
	if err != nil {
		return defaultUploadRate
	}

	return fmt.Sprintf("--limit-upload=%d", res)
}

func (o *Option) downloadRate() string {
	var defaultDownloadRate = "--limit-download=0"
	if o.LimitDownloadRate == "" {
		return defaultDownloadRate
	}

	res, err := strconv.ParseInt(o.LimitDownloadRate, 10, 64)
	if err != nil {
		return defaultDownloadRate
	}

	return fmt.Sprintf("--limit-download=%d", res)
}

func NewRestic(ctx context.Context, name string, userName string, envs map[string]string, opt *Option) (Restic, error) {
	var commandPath, err = util.GetCommand(resticFile)
	if err != nil {
		return nil, err
	}
	var ctxRestic, cancel = context.WithCancel(ctx)
	return &resticManager{
		ctx:    ctxRestic,
		cancel: cancel,
		name:   name,
		user:   userName,
		envs:   envs,
		bin:    commandPath,
		opt:    opt,
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

func (r *resticManager) Init() (*InitSummaryOutput, error) {
	opts := cmd.CommandOptions{
		Path: r.bin,
		Args: []string{
			"init",
			PARAM_JSON_OUTPUT,
			PARAM_INSECURE_TLS,
		},
		Envs: r.envs,
	}
	c := cmd.NewCommand(r.ctx, opts)
	var errorMsg RESTIC_ERROR_MESSAGE
	var summary *InitSummaryOutput

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
				var msg = string(res)
				logger.Debugf("[restic] init %s message: %s", r.name, msg)
				if strings.Contains(msg, "Fatal: ") {
					switch {
					case strings.Contains(msg, ERROR_MESSAGE_ALREADY_INITIALIZED.Error()):
						errorMsg = ERROR_MESSAGE_ALREADY_INITIALIZED
						c.Cancel()
						return
					case
						strings.Contains(msg, ERROR_MESSAGE_UNABLE_TO_OPEN_REPOSITORY.Error()),
						strings.Contains(msg, ERROR_MESSAGE_BAD_REQUEST.Error()):
						errorMsg = ERROR_MESSAGE_TOKEN_EXPIRED
						c.Cancel()
						return
					default:
						errorMsg = RESTIC_ERROR_MESSAGE(msg)
						c.Cancel()
						return
					}
				}
				if err := json.Unmarshal(res, &summary); err != nil {
					errorMsg = RESTIC_ERROR_MESSAGE(err.Error())
					c.Cancel()
					return
				}
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

func (r *resticManager) Backup(name string, folder string, filePathPrefix string) (*SummaryOutput, error) {
	var backupCtx, cancel = context.WithCancel(r.ctx)
	defer cancel()
	opts := cmd.CommandOptions{
		Path: r.bin,
		Args: []string{
			"backup",
			folder,
			r.opt.uploadRate(),
			PARAM_JSON_OUTPUT,
			PARAM_INSECURE_TLS,
		},
		Envs: r.envs,
	}

	opts.Args = append(opts.Args, r.withTag(name)...)

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
					var msg = string(res)
					logger.Debugf("[restic] backup %s error message: %s", r.name, msg)
					messagePool.Put(status)
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
						logger.Infof(PRINT_START_MESSAGE, status.TotalFiles, util.FormatBytes(status.TotalBytes))
					case math.Abs(status.PercentDone-1.0) < tolerance:
						if !finished {
							logger.Infof(PRINT_FINISH_MESSAGE, status.TotalFiles, util.FormatBytes(status.TotalBytes))
							finished = true
						}
					default:
						if prevPercent != status.PercentDone {
							logger.Infof(PRINT_PROGRESS_MESSAGE,
								status.GetPercentDone(),
								status.FilesDone,
								status.TotalFiles,
								util.FormatBytes(status.BytesDone),
								util.FormatBytes(status.TotalBytes),
								r.fileNameTidy(status.CurrentFiles, filePathPrefix))
						}
						prevPercent = status.PercentDone
					}
				case "summary":
					if err := json.Unmarshal(res, &summary); err != nil {
						logger.Debugf("[restic] backup %s error summary unmarshal message: %s", r.name, string(res))
						messagePool.Put(status)
						errorMsg = RESTIC_ERROR_MESSAGE(err.Error())
						c.Cancel()
						return
					}
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

		if strings.Contains(res, ERROR_MESSAGE_LOCKED.Error()) {
			r.Unlock()
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
				logger.Debugf("[restic] repair %s message: %s", r.name, string(res))
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
				logger.Debugf("[restic] unlock %s message: %s", r.name, string(res))
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

func (r *resticManager) GetSnapshot(snapshotId string) (*Snapshot, error) {
	var restoreCtx, cancel = context.WithCancel(r.ctx)
	defer cancel()
	opts := cmd.CommandOptions{
		Path: r.bin,
		Args: []string{
			"snapshots",
			PARAM_JSON_OUTPUT,
			PARAM_INSECURE_TLS,
			snapshotId,
		},
		Envs: r.envs,
	}

	c := cmd.NewCommand(restoreCtx, opts)

	var summary []*Snapshot
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

				var msg = string(res)
				logger.Debugf("[restic] snapshots %s message: %s", r.name, msg)
				if strings.Contains(msg, "Fatal: ") {
					switch {
					case strings.Contains(msg, ERROR_MESSAGE_SNAPSHOT_NOT_FOUND.Error()):
						errorMsg = ERROR_MESSAGE_SNAPSHOT_NOT_FOUND
						c.Cancel()
						return
					default:
						errorMsg = RESTIC_ERROR_MESSAGE(msg)
						c.Cancel()
						return
					}
				}
				if err := json.Unmarshal(res, &summary); err != nil {
					errorMsg = RESTIC_ERROR_MESSAGE(err.Error())
					c.Cancel()
					return
				}
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

	if summary == nil || len(summary) == 0 {
		return nil, fmt.Errorf("snapshot %s not found", snapshotId)
	}

	return summary[0], nil
}

func (r *resticManager) Restore(snapshotId string, uploadPath string, target string) (*RestoreSummaryOutput, error) {
	var restoreCtx, cancel = context.WithCancel(r.ctx)
	defer cancel()
	opts := cmd.CommandOptions{
		Path: r.bin,
		Args: []string{
			"restore",
			r.opt.downloadRate(),
			"-t",
			target,
			"-v=3",
			PARAM_JSON_OUTPUT,
			PARAM_INSECURE_TLS,
			fmt.Sprintf("%s:%s", snapshotId, uploadPath),
		},
		Envs: r.envs,
	}

	c := cmd.NewCommand(restoreCtx, opts)

	var prevPercent float64
	var started bool
	var finished bool
	var summary *RestoreSummaryOutput
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

				status := restoreMessagePool.Get()
				if err := json.Unmarshal(res, status); err != nil {
					var msg = string(res)
					logger.Debugf("[restic] restore %s error message: %s", r.name, msg)
					restoreMessagePool.Put(status)

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
						if !started {
							logger.Infof(PRINT_RESTORE_START_MESSAGE, status.TotalFiles, util.FormatBytes(status.TotalBytes))
							started = true
						}
					case math.Abs(status.PercentDone-1.0) < tolerance:
						if !finished {

							logger.Infof(PRINT_RESTORE_FINISH_MESSAGE, snapshotId, status.TotalFiles, status.FilesRestored, util.FormatBytes(status.TotalBytes), util.FormatBytes(status.BytesRestored))
							finished = true
						}
					default:
						if prevPercent != status.PercentDone {
							logger.Infof(PRINT_RESTORE_PROGRESS_MESSAGE,
								status.GetPercentDone(),
								status.FilesRestored,
								status.TotalFiles,
								util.FormatBytes(status.BytesRestored),
								util.FormatBytes(status.TotalBytes),
							)
						}
						prevPercent = status.PercentDone
					}
				case "verbose_status":
					rvu := new(RestoreVerboseUpdate)
					if err := json.Unmarshal(res, &rvu); err != nil {
						errorMsg = RESTIC_ERROR_MESSAGE(err.Error())
						c.Cancel()
						return
					}
					logger.Infof(PRINT_RESTORE_ITEM, rvu.Item, util.FormatBytes(rvu.Size))
				case "summary":
					if err := json.Unmarshal(res, &summary); err != nil {
						logger.Debugf("[restic] restore %s error summary unmarshal message: %s", r.name, string(res))
						restoreMessagePool.Put(status)
						errorMsg = RESTIC_ERROR_MESSAGE(err.Error())
						c.Cancel()
						return
					}
					restoreMessagePool.Put(status)
					return
				}
				restoreMessagePool.Put(status)
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

func (r *resticManager) withTag(name string) []string {
	return []string{"--tag", fmt.Sprintf("name=%s", name)}
}

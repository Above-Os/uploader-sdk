package storage

import (
	"bytes"
	"context"
	"crypto/tls"
	"log"
	"path"
	"strconv"
	"syscall"

	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"bytetrade.io/web3os/backups-sdk/pkg/client"
	"bytetrade.io/web3os/backups-sdk/pkg/common"
	"bytetrade.io/web3os/backups-sdk/pkg/response"
	"bytetrade.io/web3os/backups-sdk/pkg/util"
	"bytetrade.io/web3os/backups-sdk/pkg/util/logger"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"golang.org/x/term"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

type OlaresSpace struct {
	RepoName     string `json:"repo_name"`
	RepoPassword string `json:"-"`
	OlaresId     string `json:"olares_id"`
	OlaresName   string `json:"olares_name"`

	UserId    string `json:"user_id"`
	UserToken string `json:"user_token"`

	BackupType        string `json:"backup_type"`
	Endpoint          string `json:"endpoint"`
	AccessKeyId       string `json:"access_key_id"`
	SecretAccessKey   string `json:"secret_access_key"`
	BackupToLocalPath string `json:"backup_to_local_path"`

	Path               string              `json:"path"`
	CloudApiMirror     string              `json:"cloud_api_mirror"`
	Duration           string              `json:"duration"`
	OlaresSpaceSession *OlaresSpaceSession `json:"olares_space_session"`
	Env                map[string]string   `json:"-"`

	BackupsOperate BackupsOperate `json:"backup_operate"`
}

type OlaresSpaceSession struct {
	Cloud          string `json:"cloud"`
	Bucket         string `json:"bucket"`
	Token          string `json:"st"`
	Prefix         string `json:"prefix"`
	Secret         string `json:"secret"`
	Key            string `json:"key"`
	Expiration     string `json:"expiration"`
	Region         string `json:"region"`
	ResticRepo     string `json:"restic_repo"`
	ResticPassword string `json:"-"`
}

type AccountResponse struct {
	response.Header
	Data *AccountResponseData `json:"data,omitempty"`
}

type AccountResponseRawData struct {
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	// ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt int64  `json:"expires_at"`
	UserId    string `json:"userid"`
	Available bool   `json:"available"`
	CreateAt  int64  `json:"create_at"`
}

type AccountResponseData struct {
	Name     string                  `json:"name"`
	Type     string                  `json:"type"`
	RawData  *AccountResponseRawData `json:"raw_data"`
	CloudUrl string                  `json:"cloudUrl"`
}

type AccountValue struct {
	Email   string `json:"email"`
	Userid  string `json:"userid"`
	Token   string `json:"token"`
	Expired any    `json:"expired"`
}

var UsersGVR = schema.GroupVersionResource{
	Group:    "iam.kubesphere.io",
	Version:  "v1alpha2",
	Resource: "users",
}

func (c *OlaresSpaceSession) Expire() (time.Time, error) {
	return time.Parse(time.RFC3339, c.Expiration)
}

type CloudStorageAccountResponse struct {
	response.Header
	Data *OlaresSpaceSession `json:"data"`
}

func (t *OlaresSpace) SetRepoUrl() {
	switch t.BackupType {
	case common.BackupTypeS3:
		t.formatS3Repo()
	case common.BackupTypeCos:
		t.formatCosRepo()
	case common.BackupTypeLocal:
		t.formatLocalRepo()
	default:
		t.formatOlaresSpaceRepo() // todo distinguish between s3 and cos
	}
}

func (t *OlaresSpace) formatOlaresSpaceRepo() {
	// todo distinguish between s3 and cos
	var repoPrefix = filepath.Join(t.OlaresSpaceSession.Prefix, "restic", t.RepoName)
	var domain = fmt.Sprintf("s3.%s.%s", t.OlaresSpaceSession.Region, common.AwsDomain)
	var repo = filepath.Join(domain, t.OlaresSpaceSession.Bucket, repoPrefix)
	var repoUrl = fmt.Sprintf("s3:%s", repo)

	t.OlaresSpaceSession.ResticRepo = repoUrl
	t.OlaresSpaceSession.ResticPassword = t.RepoPassword
}

func (t *OlaresSpace) formatS3Repo() error {
	if t.Endpoint == "" {
		return fmt.Errorf("endpoint is empty")
	}
	var endpoint = strings.TrimPrefix(t.Endpoint, "https://")
	endpoint = strings.TrimRight(endpoint, "/")
	if strings.EqualFold(endpoint, "") {
		return fmt.Errorf("endpoint is invalid")
	}

	var repoSplit = strings.SplitN(endpoint, "/", 2)
	if repoSplit == nil || len(repoSplit) < 1 {
		return fmt.Errorf("endpoint is invalid")
	}
	var repoBase = repoSplit[0]
	var repoPrefix = ""
	if len(repoSplit) >= 2 {
		repoPrefix = fmt.Sprintf("%s/", repoSplit[1])
	}

	var repoBaseSplit = strings.SplitN(repoBase, ".", 3)
	if len(repoBaseSplit) != 3 {
		return fmt.Errorf("endpoint is invalid")
	}
	if repoBaseSplit[2] != common.AwsDomain {
		return fmt.Errorf("endpoint is invalid")
	}
	var bucket = repoBaseSplit[0]
	var region = repoBaseSplit[1]

	t.OlaresSpaceSession.Key = t.AccessKeyId
	t.OlaresSpaceSession.Secret = t.SecretAccessKey
	t.OlaresSpaceSession.Region = region
	t.OlaresSpaceSession.ResticRepo = fmt.Sprintf("s3:s3.%s.%s/%s/%s%s", region, common.AwsDomain, bucket, repoPrefix, t.RepoName)
	t.OlaresSpaceSession.ResticPassword = t.RepoPassword

	return nil
}

func (t *OlaresSpace) formatCosRepo() error {
	if t.Endpoint == "" {
		return fmt.Errorf("endpoint is empty")
	}
	var endpoint = strings.TrimPrefix(t.Endpoint, "https://")
	endpoint = strings.TrimRight(endpoint, "/")
	if strings.EqualFold(endpoint, "") {
		return fmt.Errorf("endpoint is invalid")
	}

	var repoSplit = strings.Split(endpoint, "/")
	if repoSplit == nil || len(repoSplit) < 2 {
		return fmt.Errorf("endpoint is invalid")
	}

	var repoBase = repoSplit[0]
	var repoBucket = repoSplit[1]
	var repoPrefix = ""
	if len(repoSplit) > 2 {
		repoPrefix = fmt.Sprintf("%s/", strings.Join(repoSplit[2:], "/"))
	}

	var repoBaseSplit = strings.SplitN(repoBase, ".", 3)
	if repoBaseSplit == nil || len(repoBaseSplit) != 3 {
		return fmt.Errorf("endpoint is invalid")
	}
	if repoBaseSplit[0] != "cos" || repoBaseSplit[2] != common.TencentDomain {
		return fmt.Errorf("endpoint is invalid")
	}
	var repoRegion = repoBaseSplit[1]

	t.OlaresSpaceSession.Key = t.AccessKeyId
	t.OlaresSpaceSession.Secret = t.SecretAccessKey
	t.OlaresSpaceSession.Region = repoRegion
	t.OlaresSpaceSession.ResticRepo = fmt.Sprintf("s3:https://cos.%s.%s/%s/%s%s", repoRegion, common.TencentDomain, repoBucket, repoPrefix, t.RepoName)
	t.OlaresSpaceSession.ResticPassword = t.RepoPassword

	return nil
}

func (t *OlaresSpace) formatLocalRepo() error {
	t.OlaresSpaceSession.ResticRepo = path.Join(t.BackupToLocalPath, t.RepoName)
	t.OlaresSpaceSession.ResticPassword = t.RepoPassword
	return nil
}

func (s *OlaresSpace) EnterPassword() error {
	if s.BackupsOperate.IsBackup() {
		fmt.Println("\nPlease create a password for this backup. This password will be required to restore your data in the future. The system will NOT save or store this password, so make sure to remember it. If you lose or forget this password, you will not be able to recover your backup.")
	}
	var password []byte
	var confirmed []byte
	_ = password

	for {
		fmt.Print("\nEnter password for repository: ")
		password, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatalf("Failed to read password: %v", err)
			return err
		}
		password = bytes.TrimSpace(password)
		if len(password) == 0 {
			continue
		}
		confirmed = password
		if !s.BackupsOperate.IsBackup() {
			break
		}
		fmt.Print("\nRe-enter the password to confirm: ")
		confirmed, err = term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatalf("Failed to read re-enter password: %v", err)
			return err
		}
		if !bytes.Equal(password, confirmed) {
			fmt.Printf("\nPasswords do not match. Please try again.\n")
			continue
		}

		break
	}
	s.RepoPassword = string(confirmed)
	fmt.Printf("\n\n")

	return nil
}

func (t *OlaresSpace) SetEnv() {
	if t.Env == nil {
		t.Env = make(map[string]string)
	}

	t.Env["AWS_ACCESS_KEY_ID"] = t.OlaresSpaceSession.Key
	t.Env["AWS_SECRET_ACCESS_KEY"] = t.OlaresSpaceSession.Secret
	t.Env["AWS_SESSION_TOKEN"] = t.OlaresSpaceSession.Token
	t.Env["RESTIC_REPOSITORY"] = t.OlaresSpaceSession.ResticRepo
	t.Env["RESTIC_PASSWORD"] = t.OlaresSpaceSession.ResticPassword

	msg := fmt.Sprintf("export AWS_ACCESS_KEY_ID=%s\nexport AWS_SECRET_ACCESS_KEY=%s\nexport AWS_SESSION_TOKEN=%s\nexport RESTIC_REPOSITORY=%s\nexport AWS_REGION=%s\n",
		t.OlaresSpaceSession.Key,
		t.OlaresSpaceSession.Secret,
		t.OlaresSpaceSession.Token,
		t.OlaresSpaceSession.ResticRepo,
		t.OlaresSpaceSession.Region,
	)

	_ = msg
	// fmt.Println(msg)
	// logger.Debugf("export env: %s", util.Base64encode([]byte(msg)))
}

func (t *OlaresSpace) GetEnv() map[string]string {
	return t.Env
}

func (t *OlaresSpace) RefreshToken(isDebug bool) error {
	if t.BackupType != common.DefaultBackupType {
		return nil
	}
	if t.UserId != "" && t.UserToken != "" {
		logger.Infof("retrieving olares space token, userid: %s, usertoken: %s", t.UserId, t.UserToken)
		err := t.setToken(isDebug)
		if err == nil {
			return nil
		}
		logger.Info("failed to obtain olares space token, retrying, please wait...")
	}

	podIp, err := t.getPodIp()
	if err != nil {
		return err
	}

	appKey, err := t.getAppKey()
	if err != nil {
		return err
	}

	logger.Infof("retrieving user %s token", t.OlaresId)
	userId, userToken, err := t.getUserToken(podIp, appKey)
	if err != nil {
		return err
	}
	t.UserId = userId
	t.UserToken = userToken

	return t.setToken(isDebug)
}

func (t *OlaresSpace) SetAccount() error {
	factory, err := client.NewFactory()
	if err != nil {
		return errors.WithStack(err)
	}

	dynamicClient, err := factory.DynamicClient()
	if err != nil {
		return errors.WithStack(err)
	}

	var backoff = wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    5,
	}

	var olaresName string
	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		unstructuredUser, err := dynamicClient.Resource(UsersGVR).Get(ctx, t.OlaresId, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		obj := unstructuredUser.UnstructuredContent()
		olaresName, _, err = unstructured.NestedString(obj, "spec", "email")
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}); err != nil {
		return errors.WithStack(err)
	}

	t.OlaresName = olaresName
	return nil
}

func (t *OlaresSpace) getPodIp() (string, error) {
	factory, err := client.NewFactory()
	if err != nil {
		return "", errors.WithStack(err)
	}

	kubeClient, err := factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pods, err := kubeClient.CoreV1().Pods(fmt.Sprintf("user-system-%s", t.OlaresId)).List(ctx, metav1.ListOptions{
		LabelSelector: "app=systemserver",
	})
	if err != nil {
		return "", errors.WithStack(err)
	}

	if pods == nil || pods.Items == nil || len(pods.Items) == 0 {
		return "", fmt.Errorf("system server pod not found")
	}

	pod := pods.Items[0]
	podIp := pod.Status.PodIP
	if podIp == "" {
		return "", fmt.Errorf("system server pod ip invalid")
	}

	return podIp, nil
}

func (t *OlaresSpace) getAppKey() (string, error) {
	factory, err := client.NewFactory()
	if err != nil {
		return "", errors.WithStack(err)
	}

	kubeClient, err := factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	secret, err := kubeClient.CoreV1().Secrets("os-system").Get(ctx, "app-key", metav1.GetOptions{})
	if err != nil {
		return "", errors.WithStack(err)
	}
	if secret == nil || secret.Data == nil || len(secret.Data) == 0 {
		return "", fmt.Errorf("secret not found")
	}

	key, ok := secret.Data["random-key"]
	if !ok {
		return "", fmt.Errorf("app key not found")
	}

	return string(key), nil
}

func (t *OlaresSpace) getUserToken(podIp string, appKey string) (userid, token string, err error) {
	terminusNonce, err := util.GenTerminusNonce(appKey)
	if err != nil {
		logger.Errorf("generate nonce error: %v", err)
		return
	}
	var settingsUrl = fmt.Sprintf("http://%s/legacy/v1alpha1/service.settings/v1/api/account/retrieve", podIp)

	client := resty.New().SetTimeout(10 * time.Second)
	var data = make(map[string]string)
	data["name"] = fmt.Sprintf("integration-account:space:%s", t.OlaresName)
	logger.Infof("fetch account from settings: %s", settingsUrl)
	resp, err := client.R().SetDebug(true).
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", terminusNonce).
		SetBody(data).
		SetResult(&AccountResponse{}).
		Post(settingsUrl)

	if err != nil {
		return
	}

	if resp.StatusCode() != http.StatusOK {
		err = errors.WithStack(fmt.Errorf("request settings account api response not ok, status: %d", resp.StatusCode()))
		return
	}

	accountResp := resp.Result().(*AccountResponse)

	if accountResp.Code == 1 && accountResp.Message == "" {
		err = errors.WithStack(fmt.Errorf("\nOlares Space is not enabled. Please go to the Settings - Integration page in the LarePass App to add Space\n"))
		return
	} else if accountResp.Code != 0 {
		err = errors.WithStack(fmt.Errorf("request settings account api response error, status: %d, message: %s", accountResp.Code, accountResp.Message))
		return
	}

	if accountResp.Data == nil || accountResp.Data.RawData == nil {
		err = errors.WithStack(fmt.Errorf("request settings account api response data is nil, status: %d, message: %s", accountResp.Code, accountResp.Message))
		return
	}

	if accountResp.Data.RawData.UserId == "" || accountResp.Data.RawData.AccessToken == "" {
		err = errors.WithStack(fmt.Errorf("access token invalid"))
		return
	}

	userid = accountResp.Data.RawData.UserId
	token = accountResp.Data.RawData.AccessToken

	return
}

func (t *OlaresSpace) GetToken() {
	t.setToken(false)
}

func (t *OlaresSpace) setToken(isDebug bool) error {
	var backoff = wait.Backoff{
		Duration: 3 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    3,
	}

	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		var serverDomain = util.DefaultValue(common.DefaultCloudApiUrl, t.CloudApiMirror)

		serverURL := fmt.Sprintf("%s/v1/resource/stsToken/backup", strings.TrimRight(serverDomain, "/"))

		httpClient := resty.New().SetTimeout(15 * time.Second).SetDebug(true).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
		resp, err := httpClient.R().
			SetFormData(map[string]string{
				"userid":          t.UserId,
				"token":           t.UserToken,
				"cloudName":       t.parseCloudName(),
				"region":          t.parseRegion(),
				"clusterId":       util.MD5(t.Path),
				"durationSeconds": fmt.Sprintf("%.0f", t.parseDuration().Seconds()),
			}).
			SetResult(&CloudStorageAccountResponse{}).
			Post(serverURL)

		if err != nil {
			return errors.WithStack(fmt.Errorf("fetch data from cloud error: %v, url: %s", err, serverURL))
		}

		if resp.StatusCode() != http.StatusOK {
			return errors.WithStack(fmt.Errorf("fetch data from cloud response error: %d, data: %s", resp.StatusCode(), resp.Body()))
		}

		queryResp := resp.Result().(*CloudStorageAccountResponse)
		if queryResp.Code != http.StatusOK { // 506
			return errors.WithStack(fmt.Errorf("get cloud storage account from cloud error: %d, data: %s",
				queryResp.Code, queryResp.Message))
		}

		if queryResp.Data == nil {
			return errors.WithStack(fmt.Errorf("get cloud storage account from cloud data is empty, code: %d, data: %s", queryResp.Code, queryResp.Message))
		}

		t.OlaresSpaceSession = queryResp.Data

		if isDebug {
		}

		return nil
	}); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (t *OlaresSpace) parseRegion() string {
	if t.BackupType == common.DefaultBackupType {
		return common.DefaultBackupOlaresRegion
	}
	return ""
}

func (t *OlaresSpace) parseCloudName() string {
	switch t.BackupType {
	case common.BackupTypeCos:
		return common.BackupTypeCos
	case common.BackupTypeS3:
		return common.BackupTypeS3
	default:
		return common.BackupTypeOlaresAWS
	}
}

func (t *OlaresSpace) parseDuration() time.Duration {
	var defaultDuration = 12 * time.Hour
	if t.Duration == "" {
		return defaultDuration
	}

	res, err := strconv.ParseInt(t.Duration, 10, 64)
	if err != nil {
		return defaultDuration
	}
	dur, err := time.ParseDuration(fmt.Sprintf("%dm", res))
	if err != nil {
		return defaultDuration
	}

	return dur
}

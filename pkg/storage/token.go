package storage

import (
	"context"
	"crypto/tls"

	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"bytetrade.io/web3os/uploader-sdk/pkg/client"
	"bytetrade.io/web3os/uploader-sdk/pkg/common"
	"bytetrade.io/web3os/uploader-sdk/pkg/response"
	"bytetrade.io/web3os/uploader-sdk/pkg/util"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

type OlaresSpace struct {
	UserName    string `json:"user_name"`
	AccountName string `json:"account_name"`

	UserId    string `json:"user_id"`
	UserToken string `json:"user_token"`

	CloudName          string              `json:"cloud_name"`
	CloudRegion        string              `json:"cloud_region"`
	UploadPath         string              `json:"upload_path"`
	CloudApiMirror     string              `json:"cloud_api_mirror"`
	Duration           time.Duration       `json:"duration"`
	OlaresSpaceSession *OlaresSpaceSession `json:"olares_space_session"`
	Env                map[string]string   `json:"env"`
}

type OlaresSpaceSession struct {
	Cloud      string `json:"cloud"` // "aws", "tencentcloud"
	Bucket     string `json:"bucket"`
	Token      string `json:"st"`
	Prefix     string `json:"prefix"` // "fbcf5f573ed242c28758-342957450633", "did:key:???-55c06979be5e"
	Secret     string `json:"sk"`
	Key        string `json:"ak"`
	Expiration string `json:"expiration"` // "1705550635000",
	Region     string `json:"region"`     // "us-west-1", "ap-beijing"
	RepoUrl    string `json:"repo_url"`
	Password   string `json:"password"`
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

func (t *OlaresSpace) SetRepoUrl(name, password string) {
	// repoName = <name>_<uid>
	var repoPrefix = filepath.Join(t.OlaresSpaceSession.Prefix, "restic", name)
	var domain = fmt.Sprintf("s3.%s.amazonaws.com", t.OlaresSpaceSession.Region)
	var repo = filepath.Join(domain, t.OlaresSpaceSession.Bucket, repoPrefix)
	var repoUrl = fmt.Sprintf("s3:%s", repo)

	t.OlaresSpaceSession.RepoUrl = repoUrl
	t.OlaresSpaceSession.Password = password
}

func (t *OlaresSpace) SetEnv() {
	if t.Env == nil {
		t.Env = make(map[string]string)
	}

	t.Env["AWS_ACCESS_KEY_ID"] = t.OlaresSpaceSession.Key
	t.Env["AWS_SECRET_ACCESS_KEY"] = t.OlaresSpaceSession.Secret
	t.Env["AWS_SESSION_TOKEN"] = t.OlaresSpaceSession.Token
	t.Env["RESTIC_REPOSITORY"] = t.OlaresSpaceSession.RepoUrl
	t.Env["RESTIC_PASSWORD"] = t.OlaresSpaceSession.Password

	fmt.Printf("export AWS_ACCESS_KEY_ID=%s\nexport AWS_SECRET_ACCESS_KEY=%s\nexport AWS_SESSION_TOKEN=%s\nexport RESTIC_REPOSITORY=%s\nexport RESTIC_PASSWORD=%s\nexport AWS_REGION=%s\n",
		t.OlaresSpaceSession.Key,
		t.OlaresSpaceSession.Secret,
		t.OlaresSpaceSession.Token,
		t.OlaresSpaceSession.RepoUrl,
		t.OlaresSpaceSession.Password,
		t.OlaresSpaceSession.Region,
	)
}

func (t *OlaresSpace) GetEnv() map[string]string {
	return t.Env
}

func (t *OlaresSpace) RefreshToken(isDebug bool) error {
	if t.UserId != "" && t.UserToken != "" {
		err := t.setToken(isDebug)
		if err == nil {
			return nil
		}
		fmt.Println("set token error", err)
	}

	podIp, err := t.getPodIp()
	if err != nil {
		return err
	}

	appKey, err := t.getAppKey()
	if err != nil {
		return err
	}

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

	var accountName string
	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		unstructuredUser, err := dynamicClient.Resource(UsersGVR).Get(ctx, t.UserName, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		obj := unstructuredUser.UnstructuredContent()
		accountName, _, err = unstructured.NestedString(obj, "spec", "email")
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}); err != nil {
		return errors.WithStack(err)
	}

	t.AccountName = accountName
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

	pods, err := kubeClient.CoreV1().Pods(fmt.Sprintf("user-system-%s", t.UserName)).List(ctx, metav1.ListOptions{
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
		fmt.Println("generate nonce error, ", err)
		return
	}
	var settingsUrl = fmt.Sprintf("http://%s/legacy/v1alpha1/service.settings/v1/api/account/retrieve", podIp)

	client := resty.New().SetTimeout(10 * time.Second)
	var data = make(map[string]string)
	data["name"] = fmt.Sprintf("integration-account:space:%s", t.AccountName)
	fmt.Println("fetch account from settings, ", settingsUrl)
	resp, err := client.R().SetDebug(true).
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", terminusNonce).
		SetBody(data).
		SetResult(&AccountResponse{}).
		Post(settingsUrl)

	if err != nil {
		fmt.Println("request settings account api error, ", err)
		return
	}

	if resp.StatusCode() != http.StatusOK {
		fmt.Println("request settings account api response not ok, ", resp.StatusCode())
		return
	}

	accountResp := resp.Result().(*AccountResponse)

	if accountResp.Code != 0 {
		fmt.Println("request settings account api response error, ", accountResp.Code, ", ", accountResp.Message)
		return
	}

	if accountResp.Data == nil || accountResp.Data.RawData == nil {
		fmt.Println("request settings account api response data is nil, ", accountResp.Code, ", ", accountResp.Message)
		return
	}

	if accountResp.Data.RawData.UserId == "" || accountResp.Data.RawData.AccessToken == "" {
		fmt.Println("access token invalid")
		return
	}

	userid = accountResp.Data.RawData.UserId
	token = accountResp.Data.RawData.AccessToken

	return
}

func (t *OlaresSpace) setToken(isDebug bool) error {
	var serverDomain = util.DefaultValue(common.DefaultCloudApiUrl, t.CloudApiMirror)

	serverURL := fmt.Sprintf("%s/v1/resource/stsToken/backup", strings.TrimRight(serverDomain, "/"))

	httpClient := resty.New().SetTimeout(15 * time.Second).SetDebug(true).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	resp, err := httpClient.R().
		SetFormData(map[string]string{
			"userid":          t.UserId,
			"token":           t.UserToken,
			"cloudName":       t.CloudName,
			"region":          t.CloudRegion,
			"clusterId":       util.MD5(t.UploadPath),
			"durationSeconds": fmt.Sprintf("%.0f", t.Duration.Seconds()),
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
	if queryResp.Code != http.StatusOK {
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
}

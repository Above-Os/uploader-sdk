package client

import (
	"github.com/pkg/errors"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	k8scheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	kbclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Factory interface {
	ClientConfig() (*rest.Config, error)

	DynamicClient() (dynamic.Interface, error)

	KubeClient() (kubernetes.Interface, error)

	KubeBuilderClient() (kbclient.Client, error)
}

var _ Factory = &factory{}

type factory struct {
	config *rest.Config

	client dynamic.Interface
}

func NewFactory() (Factory, error) {
	config, err := ctrl.GetConfig()
	if err != nil {
		return nil, errors.Errorf("new rest kubeconfig: %v", err)
	}

	config.Burst = 15
	config.QPS = 50

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, errors.Errorf("new dynamic client: %v", err)
	}

	f := &factory{
		config: config,
		client: client,
	}
	return f, nil
}

func (f *factory) ClientConfig() (*rest.Config, error) {
	return f.config, nil
}

func (f *factory) DynamicClient() (dynamic.Interface, error) {
	return f.client, nil
}

func (f *factory) KubeClient() (kubernetes.Interface, error) {
	c, err := kubernetes.NewForConfig(f.config)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c, nil
}

func (f *factory) KubeBuilderClient() (kbclient.Client, error) {
	var err error

	scheme := runtime.NewScheme()

	if err = k8scheme.AddToScheme(scheme); err != nil {
		return nil, errors.Errorf("add schema k8scheme: %v", err)
	}
	if err = apiextv1beta1.AddToScheme(scheme); err != nil {
		return nil, errors.Errorf("add schema apiextv1beta1: %v", err)
	}
	if err = apiextv1.AddToScheme(scheme); err != nil {
		return nil, errors.Errorf("add schema apiextv1: %v", err)
	}
	kubebuilderClient, err := kbclient.New(f.config, kbclient.Options{
		Scheme: scheme,
	})

	if err != nil {
		return nil, errors.Errorf("new kubeclient with scheme: %v", err)
	}

	return kubebuilderClient, nil
}

func (f *factory) NewUnstructuredResources() *unstructured.UnstructuredList {
	r := new(unstructured.UnstructuredList)
	r.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "List"})

	return r
}

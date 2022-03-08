package kubectl

import (
	"fmt"
	"io/ioutil"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	serializeryaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

var decUnstructured = serializeryaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

type Client struct {
	restMapper meta.RESTMapper
	kubeClient dynamic.Interface
}

func NewClient(kubeConfig string) (*Client, error) {
	kc, err := ioutil.ReadFile(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("reading kubeConfig, %w", err)
	}
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kc)
	if err != nil {
		return nil, fmt.Errorf("creating rest client, %w", err)
	}
	dc, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating discovery client, %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	return &Client{
		restMapper: mapper,
		kubeClient: dyn,
	}, nil
}

package kubectl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"knative.dev/pkg/logging"
)

func (c *Client) Apply(ctx context.Context, file string) error {
	resp, err := http.Get(file)
	if err != nil {
		return fmt.Errorf("getting response, %w", err)
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("parsing response, %w", err)
	}
	if err := c.ApplyYAML(ctx, data); err != nil {
		return fmt.Errorf("applying yaml, %w", err)
	}
	logging.FromContext(ctx).Infof("Applied %s", file)
	return nil
}

func (c *Client) ApplyYAML(ctx context.Context, yamlBytes []byte) error {
	reader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewBuffer(yamlBytes)))
	for {
		b, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading yaml, %w", err)
		}
		if len(b) == 0 {
			break
		}
		if err := c.ApplyObject(ctx, b); err != nil {
			return fmt.Errorf("applying object, %w", err)
		}
	}
	return nil
}

func (c *Client) ApplyObject(ctx context.Context, yamlBytes []byte) error {
	obj := &unstructured.Unstructured{}
	_, gvk, err := decUnstructured.Decode([]byte(yamlBytes), nil, obj)
	if err != nil {
		return fmt.Errorf("decoding object, %w", err)
	}
	mapping, err := c.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("parsing rest mapping, %w", err)
	}
	dr := dynamic.ResourceInterface(c.kubeClient.Resource(mapping.Resource))
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		dr = c.kubeClient.Resource(mapping.Resource).Namespace(obj.GetNamespace())
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("marshalling object, %w", err)
	}
	if _, err := dr.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, v1.PatchOptions{FieldManager: "kitctl"}); err != nil {
		return fmt.Errorf("patching %s, %w", obj.GetName(), err)
	}
	return nil
}

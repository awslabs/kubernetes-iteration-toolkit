/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package instances

import (
	"context"
	"fmt"
	"strings"

	"github.com/awslabs/kit/operator/pkg/controllers/master"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Provider struct {
	kubeClient *kubeprovider.Client
}

func New(client *kubeprovider.Client) *Provider {
	return &Provider{kubeClient: client}
}

func (p *Provider) ControlPlaneInstancesFor(ctx context.Context, clusterName string) ([]string, error) {
	result := []string{}
	nodes, err := p.nodesWithLabelFor(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes.Items {
		instanceID, err := parseInstanceID(node.Spec.ProviderID)
		if err != nil {
			return nil, err
		}
		result = append(result, instanceID)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("failed to find any control plane instances")
	}
	return result, nil
}

func (p *Provider) nodesWithLabelFor(ctx context.Context, clusterName string) (*v1.NodeList, error) {
	nodes := &v1.NodeList{}
	if err := p.kubeClient.List(ctx, nodes, client.MatchingLabels(master.APIServerLabels(clusterName))); err != nil {
		return nil, fmt.Errorf("getting kube nodes for cluster %v, %w", clusterName, err)
	}
	return nodes, nil
}

func parseInstanceID(providerID string) (string, error) {
	if !strings.HasPrefix(providerID, "aws:///") {
		return "", fmt.Errorf("incorrect format for provider ID, %s", providerID)
	}
	values := strings.Split(strings.TrimPrefix(providerID, "aws:///"), "/")
	if len(values) != 2 {
		return "", fmt.Errorf("parsing provider ID, %s", providerID)
	}
	return values[1], nil
}

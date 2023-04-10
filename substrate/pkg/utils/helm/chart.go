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

package helm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewClient(kubeConfig string) *Client {
	return &Client{
		kubeConfig: kubeConfig,
		httpGetter: new(getter.HTTPGetter),
	}
}

type Client struct {
	kubeConfig string
	httpGetter *getter.HTTPGetter
}
type Chart struct {
	Namespace       string
	Name            string
	Repository      string
	Version         string
	CreateNamespace bool
	Values          map[string]interface{}
}

func (c *Client) Apply(ctx context.Context, chart *Chart) (reconcile.Result, error) {
	if err := c.apply(ctx, chart); err != nil {
		if IsErrorConnectionRefused(err) || IsNetIOTimeOut(err) {
			logging.FromContext(ctx).Infof("connection refused from API server, helm chart for %s requeued", chart.Name)
			return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		}
		return reconcile.Result{}, fmt.Errorf("applying chart, %w", err)
	}
	return reconcile.Result{}, nil
}

func (c *Client) apply(ctx context.Context, chart *Chart) (err error) {
	// Get the chart from the repository
	chartPath := fmt.Sprintf("%s/%s-%s.tgz", chart.Repository, chart.Name, chart.Version)
	if chart.Version == "" {
		chartPath, err = repo.FindChartInRepoURL(chart.Repository, chart.Name, "", "", "", "", getter.All(&cli.EnvSettings{}))
		if err != nil {
			return fmt.Errorf("failed to find chart, %w", err)
		}
	}
	charts, err := c.httpGetter.Get(chartPath)
	if err != nil {
		return fmt.Errorf("getting chart, %w", err)
	}
	// Load archive file in memory and return *chart.Chart
	desiredChart, err := loader.LoadArchive(charts)
	if err != nil {
		return fmt.Errorf("loading chart archive, %w", err)
	}
	actionConfig := new(action.Configuration)
	client := &genericclioptions.ConfigFlags{KubeConfig: &c.kubeConfig, Namespace: &chart.Namespace}
	if err := actionConfig.Init(client, chart.Namespace, "", func(_ string, _ ...interface{}) {}); err != nil {
		return fmt.Errorf("init helm action config, %w", err)
	}
	// delete any pending charts, we get into this situation when we have helm applied but instance reboots.
	// error from helm upgrade -  another operation (install/upgrade/rollback) is in progress
	lastRelease, err := actionConfig.Releases.Last(chart.Name)
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return fmt.Errorf("getting last release info for %s, %w", chart.Name, err)
	}
	if !errors.Is(err, driver.ErrReleaseNotFound) && lastRelease.Info.Status.IsPending() {
		logging.FromContext(ctx).Infof("helm chart %s is currently pending, uninstalling", chart.Name)
		deleteClient := action.NewUninstall(actionConfig)
		if _, err := deleteClient.Run(chart.Name); err != nil {
			return fmt.Errorf("getting last release info for %s, %w", chart.Name, err)
		}
	}
	// check history for the releaseName, if release is not found install else upgrade
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	if _, err := histClient.Run(chart.Name); err == driver.ErrReleaseNotFound {
		installClient := action.NewInstall(actionConfig)
		installClient.Namespace = chart.Namespace
		installClient.ReleaseName = chart.Name
		installClient.CreateNamespace = chart.CreateNamespace
		installClient.Wait = true
		installClient.Timeout = time.Second * 180
		if _, err := installClient.Run(desiredChart, chart.Values); err != nil {
			return fmt.Errorf("installing chart: %w", err)
		}
		logging.FromContext(ctx).Infof("Installed chart %s/%s", chart.Repository, chart.Name)
		return nil
	}
	upgradeClient := action.NewUpgrade(actionConfig)
	upgradeClient.Namespace = chart.Namespace
	upgradeClient.Wait = true
	upgradeClient.Timeout = time.Second * 180
	if _, err := upgradeClient.Run(chart.Name, desiredChart, chart.Values); err != nil {
		return fmt.Errorf("upgrading chart: %w", err)
	}
	logging.FromContext(ctx).Infof("Upgraded chart %s/%s", chart.Repository, chart.Name)
	return nil
}

func IsErrorConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}

func IsNetIOTimeOut(err error) bool {
	netErr := net.Error(nil)
	return errors.As(err, &netErr) && netErr.Timeout()
}

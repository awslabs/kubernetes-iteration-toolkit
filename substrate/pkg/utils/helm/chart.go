package helm

import (
	"context"
	"fmt"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"knative.dev/pkg/logging"
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

func (c *Client) Apply(ctx context.Context, chart *Chart) error {
	// Get the chart from the repository
	charts, err := c.httpGetter.Get(fmt.Sprintf("%s/%s-%s.tgz", chart.Repository, chart.Name, chart.Version))
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

package substrate

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"knative.dev/pkg/logging"
)

func NewSession() *session.Session {
	sess := session.Must(session.NewSession(&aws.Config{STSRegionalEndpoint: endpoints.RegionalSTSEndpoint}))
	sess.Handlers.Build.PushBack(request.MakeAddToUserAgentFreeFormHandler("kit.sh"))
	return sess
}

func NewController(ctx context.Context) *Controller {
	session := NewSession()
	ec2Client := ec2.New(session)
	iamClient := iam.New(session)
	ssmClient := ssm.New(session)
	autoscalingClient := autoscaling.New(session)

	routeTable := &routeTable{ec2Client}
	vpc := &vpc{ec2Client}
	subnet := &subnet{ec2Client}
	apiServer := &KubeAPIServer{}
	return &Controller{
		Resources: []Resource{
			&iamRole{iamClient},
			&iamPolicy{iamClient},
			&iamProfile{iamClient},
			vpc,
			&elasticIPForNatGW{&elasticIP{ec2Client}},
			&elasticIPForAPIServer{&elasticIP{ec2Client}},
			&internetGateway{ec2Client, vpc},
			subnet,
			&securityGroup{ec2Client},
			&natGateway{ec2Client},
			routeTable,
			&routeTableAssociation{ec2Client, routeTable},
			&launchTemplate{ec2Client, ssmClient, apiServer, session.Config.Region},
			&autoScalingGroup{ec2Client, autoscalingClient, subnet},
			apiServer,
		},
		KubeAPIServer: apiServer,
	}
}

type Controller struct {
	Resources     []Resource
	KubeAPIServer *KubeAPIServer
}

type Resource interface {
	Create(context.Context, *v1alpha1.Substrate) error
	Delete(context.Context, *v1alpha1.Substrate) error
}

func (c *Controller) Reconcile(ctx context.Context, substrate *v1alpha1.Substrate) error {
	logging.FromContext(ctx).Infof("Reconciling resources for %s", substrate.Name)
	start := time.Now()
	for _, resource := range c.Resources {
		if err := resource.Create(ctx, substrate); err != nil {
			return fmt.Errorf("failed to create resource, %w", err)
		}
	}
	logging.FromContext(ctx).Infof("Succeeded after %s", time.Since(start))
	return nil
}

func (c *Controller) Finalize(ctx context.Context, substrate *v1alpha1.Substrate) error {
	logging.FromContext(ctx).Infof("Finalizing resources for %s", substrate.Name)
	start := time.Now()
	for _, resource := range reverse(c.Resources) {
		if err := resource.Delete(ctx, substrate); err != nil {
			return fmt.Errorf("failed to create a resource, %w", err)
		}
	}
	logging.FromContext(ctx).Infof("Succeeded after %s", time.Since(start))
	// create the kubeconfig file for this substrate cluster
	return nil
}

func reverse(resources []Resource) []Resource {
	for i, j := 0, len(resources)-1; i < j; i, j = i+1, j-1 {
		resources[i], resources[j] = resources[j], resources[i]
	}
	return resources
}

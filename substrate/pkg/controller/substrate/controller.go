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

package substrate

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate/cluster"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate/cluster/addons"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate/infrastructure"
	"github.com/imdario/mergo"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewController(ctx context.Context) *Controller {
	session := session.Must(session.NewSession(&aws.Config{STSRegionalEndpoint: endpoints.RegionalSTSEndpoint}))
	session.Handlers.Build.PushBack(request.MakeAddToUserAgentFreeFormHandler("kit.sh"))
	EC2 := ec2.New(session)
	IAM := iam.New(session)
	return &Controller{
		Resources: []Resource{
			&infrastructure.VPC{EC2: EC2},
			&infrastructure.Subnets{EC2: EC2},
			&infrastructure.RouteTable{EC2: EC2},
			&infrastructure.InternetGateway{EC2: EC2},
			&infrastructure.SecurityGroup{EC2: EC2},
			&cluster.Address{EC2: EC2},
			&cluster.LaunchTemplate{EC2: EC2, SSM: ssm.New(session), Region: session.Config.Region},
			&cluster.InstanceProfile{IAM: IAM},
			&cluster.Instance{EC2: EC2},
			&cluster.Config{S3: s3.New(session), STS: sts.New(session), S3Uploader: s3manager.NewUploader(session)},
			&addons.RBAC{},
			&addons.KubeProxy{},
		},
	}
}

type Controller struct {
	sync.RWMutex
	Resources []Resource
}

type Resource interface {
	Create(context.Context, *v1alpha1.Substrate) (reconcile.Result, error)
	Delete(context.Context, *v1alpha1.Substrate) (reconcile.Result, error)
}

func (c *Controller) Reconcile(ctx context.Context, substrate *v1alpha1.Substrate) error {
	ctx, cancel := context.WithCancel(ctx)
	var errs = make([]error, len(c.Resources))
	workqueue.ParallelizeUntil(ctx, len(c.Resources), len(c.Resources), func(i int) {
		for {
			resource := c.Resources[i]
			c.RLock()
			mutable := substrate.DeepCopy()
			c.RUnlock()
			f := resource.Create
			if substrate.DeletionTimestamp != nil {
				f = resource.Delete
			}
			result, err := f(ctx, mutable)
			if err != nil {
				errs[i] = fmt.Errorf("reconciling %s, %w", reflect.ValueOf(resource).Elem().Type(), err)
				cancel()
				return
			}
			c.Lock()
			runtime.Must(mergo.Merge(substrate, mutable))
			c.Unlock()
			if !result.Requeue && result.RequeueAfter == 0 {
				return
			}
			time.Sleep(result.RequeueAfter + time.Second*1)
		}
	})
	return multierr.Combine(errs...)
}

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

package controller

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type S3 struct {
	s3 *awsprovider.S3
}

// NewS3Service returns a controller for managing S3s in AWS
func NewS3Controller(s3 *awsprovider.S3) *S3 {
	return &S3{s3: s3}
}

// Name returns the name of the controller
func (s *S3) Name() string {
	return "S3"
}

// For returns the resource this controller is for.
func (s *S3) For() controllers.Object {
	return &v1alpha1.S3{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the S3.Status
// object
func (s *S3) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	s3Obj := object.(*v1alpha1.S3)
	// 1. Get the S3 from AWS
	bucket, err := s.getBucket(ctx, s3Obj.Spec.BucketName)
	if err != nil {
		return nil, err
	}
	// 2. If bucket doesn't exist, create a new bucket for this cluster
	if bucket == nil {
		_, err := s.s3.CreateBucket(&s3.CreateBucketInput{
			Bucket: aws.String(s3Obj.Spec.BucketName),
			CreateBucketConfiguration: &s3.CreateBucketConfiguration{
				LocationConstraint: s.s3.Config.Region,
			},
		})
		if err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully created S3 bucket %v for cluster %v", s3Obj.Spec.BucketName, s3Obj.Name)
	} else {
		zap.S().Debugf("Successfully discovered S3 bucket %v for cluster %v", s3Obj.Spec.BucketName, s3Obj.Name)
	}
	// 3. Sync resource status with the S3 status object in Kubernetes
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (s *S3) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	s3Obj := object.(*v1alpha1.S3)
	if err := s.deleteBucket(ctx, s3Obj.Name); err != nil {
		return nil, err
	}
	zap.S().Infof("Successfully deleted S3 bucket %v for cluster %v", s3Obj.Spec.BucketName, s3Obj.Name)
	return status.Terminated, nil
}

func (s *S3) getBucket(ctx context.Context, bucketName string) (*s3.Bucket, error) {
	return getBucket(ctx, s.s3, bucketName)
}

func getBucket(ctx context.Context, s3api *awsprovider.S3, bucketName string) (*s3.Bucket, error) {
	output, err := s3api.ListBucketsWithContext(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("listing S3 buckets %v, err: %w", bucketName, err)
	}
	for _, bucket := range output.Buckets {
		if aws.StringValue(bucket.Name) == bucketName {
			return bucket, nil
		}
	}
	return nil, nil
}

func (s *S3) deleteBucket(ctx context.Context, bucketName string) error {
	S3, err := s.getBucket(ctx, bucketName)
	if err != nil {
		return err
	}
	// S3 doesn't exist, return
	if S3 == nil {
		return nil
	}
	if _, err := s.s3.DeleteBucketWithContext(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	}); err != nil {
		return fmt.Errorf("deleting S3, %w", err)
	}
	return nil
}

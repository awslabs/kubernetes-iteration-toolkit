package kubeprovider

import (
	"context"

	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	client.Client
}

func New(client client.Client) *Client {
	return &Client{Client: client}
}

const (
	tlsCertName = "tls.crt"
	tlsKeyName  = "tls.key"
)

func (c *Client) CertKeyFromSecret(ctx context.Context, name, namespace string) (*secrets.CertAndKey, error) {
	secret := &v1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace},
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		return nil, err
	}
	return &secrets.CertAndKey{secret.Data[tlsCertName], secret.Data[tlsKeyName]}, nil
}

// func (c *Client) Get(ctx context.Context, obj client.Object) (client.Object, error) {
// 	return nil, nil
// }

// Ensure creates if not exist, else will update the existing object
func (c *Client) Ensure(ctx context.Context, desired client.Object) error {
	if err := c.Get(ctx, client.ObjectKeyFromObject(desired), desired.DeepCopyObject().(client.Object)); err != nil {
		if errors.IsNotFound(err) {
			return c.Create(ctx, desired)
		}
		return err
	}
	return c.Update(ctx, desired)
}

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

package secrets

import (
	pkiutil "github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/pki"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	certutil "k8s.io/client-go/util/cert"
)

type Request struct {
	*certutil.Config
	CASecret  *v1.Secret
	Type      RequestType
	Name      string
	Namespace string
}

type RequestType int

const (
	CA RequestType = iota + 1
	KeyPair
	KeyWithSignedCert
)

const (
	SecretPrivateKey = "private"
	SecretPublicKey  = "public"
	SecretConfigKey  = "config"
)

func (r *Request) Create() (secret *v1.Secret, err error) {
	var private, public []byte
	switch r.Type {
	case CA:
		private, public, err = pkiutil.RootCA(&certutil.Config{CommonName: r.CommonName})
	case KeyPair:
		private, public, err = pkiutil.GenerateKeyPair()
	case KeyWithSignedCert:
		caKey, caCert := Parse(r.CASecret)
		private, public, err = pkiutil.GenerateSignedCertAndKey(r.Config, caCert, caKey)
	}
	if err != nil {
		return nil, err
	}
	return secretObjWithKeyPair(object.NamespacedName(r.Name, r.Namespace), private, public), nil
}

func IsValid(secret *v1.Secret) error {
	// TODO
	switch secret.Type {
	case v1.SecretTypeTLS:
		// Check secret.Data
	case v1.SecretTypeOpaque:
		// Check secret.Data
	}
	return nil
}

func CreateWithConfig(nn types.NamespacedName, config []byte) client.Object {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			SecretConfigKey: config,
		},
	}
}

func Parse(secret *v1.Secret) (key, cert []byte) {
	return secret.Data[SecretPrivateKey], secret.Data[SecretPublicKey]
}

func secretObjWithKeyPair(nn types.NamespacedName, private, public []byte) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			SecretPrivateKey: private,
			SecretPublicKey:  public,
		},
	}
}

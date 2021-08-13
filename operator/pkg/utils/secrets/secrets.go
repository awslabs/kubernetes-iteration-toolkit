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
	pkiutil "github.com/awslabs/kit/operator/pkg/pki"
	"github.com/awslabs/kit/operator/pkg/utils/object"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	certutil "k8s.io/client-go/util/cert"
)

type Request struct {
	*certutil.Config
	Name      string
	Namespace string
}

const (
	SecretCertName = "tls.crt"
	SecretKeyName  = "tls.key"
)

func CreateWithCerts(config *Request, caSecret *v1.Secret) (*v1.Secret, error) {
	cert, key, err := CreateCertAndKey(config, caSecret)
	if err != nil {
		return nil, err
	}
	return secretObjWithCerts(object.NamespacedName(config.Name, config.Namespace), cert, key), nil
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

func CreateCertAndKey(config *Request, caSecret *v1.Secret) (certBytes, keyBytes []byte, err error) {
	if caSecret == nil {
		return pkiutil.RootCA(&certutil.Config{CommonName: config.CommonName})
	}
	caCert, caKey := Parse(caSecret)
	cert, key, err := pkiutil.GenerateCertAndKey(config.Config, caCert, caKey)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func CreateWithConfig(nn types.NamespacedName, config []byte) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"config": config,
		},
	}
}

func Parse(secret *v1.Secret) (cert, key []byte) {
	return secret.Data[SecretCertName], secret.Data[SecretKeyName]
}

func secretObjWithCerts(nn types.NamespacedName, cert, key []byte) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			SecretCertName: cert,
			SecretKeyName:  key,
		},
	}
}

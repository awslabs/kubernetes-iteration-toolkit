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

package addons

var pipeline = `
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: Namespace
metadata:
  name: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
  annotations:
    seccomp.security.alpha.kubernetes.io/allowedProfileNames: 'docker/default,runtime/default'
    seccomp.security.alpha.kubernetes.io/defaultProfileName: 'runtime/default'
    apparmor.security.beta.kubernetes.io/allowedProfileNames: 'runtime/default'
    apparmor.security.beta.kubernetes.io/defaultProfileName: 'runtime/default'
spec:
  privileged: false
  allowPrivilegeEscalation: false
  requiredDropCapabilities:
    - ALL
  volumes:
    - 'emptyDir'
    - 'configMap'
    - 'secret'
  hostNetwork: false
  hostIPC: false
  hostPID: false
  runAsUser:
    rule: 'MustRunAsNonRoot'
  runAsGroup:
    rule: 'MustRunAs'
    ranges:
      - min: 1
        max: 65535
  seLinux:
    rule: 'RunAsAny'
  supplementalGroups:
    rule: 'MustRunAs'
    ranges:
      - min: 1
        max: 65535
  fsGroup:
    rule: 'MustRunAs'
    ranges:
      - min: 1
        max: 65535

---
# Copyright 2020 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: tekton-pipelines-controller-cluster-access
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
rules:
  - apiGroups: [""]
    # Controller needs to watch Pods created by TaskRuns to see them progress.
    resources: ["pods"]
    verbs: ["list", "watch"]
    # Controller needs cluster access to all of the CRDs that it is responsible for
    # managing.
  - apiGroups: ["tekton.dev"]
    resources: ["tasks", "clustertasks", "taskruns", "pipelines", "pipelineruns", "pipelineresources", "conditions", "runs"]
    verbs: ["get", "list", "create", "update", "delete", "patch", "watch"]
  - apiGroups: ["tekton.dev"]
    resources: ["taskruns/finalizers", "pipelineruns/finalizers", "runs/finalizers"]
    verbs: ["get", "list", "create", "update", "delete", "patch", "watch"]
  - apiGroups: ["tekton.dev"]
    resources: ["tasks/status", "clustertasks/status", "taskruns/status", "pipelines/status", "pipelineruns/status", "pipelineresources/status", "runs/status"]
    verbs: ["get", "list", "create", "update", "delete", "patch", "watch"]
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  # This is the access that the controller needs on a per-namespace basis.
  name: tekton-pipelines-controller-tenant-access
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
rules:
  # Read-write access to create Pods and PVCs (for Workspaces)
  - apiGroups: [""]
    resources: ["pods", "persistentvolumeclaims"]
    verbs: ["get", "list", "create", "update", "delete", "patch", "watch"]
  # Write permissions to publish events.
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "update", "patch"]
  # Read-only access to these.
  - apiGroups: [""]
    resources: ["configmaps", "limitranges", "secrets", "serviceaccounts"]
    verbs: ["get", "list", "watch"]
  # Read-write access to StatefulSets for Affinity Assistant.
  - apiGroups: ["apps"]
    resources: ["statefulsets"]
    verbs: ["get", "list", "create", "update", "delete", "patch", "watch"]
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: tekton-pipelines-webhook-cluster-access
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
rules:
  # The webhook needs to be able to get and update customresourcedefinitions,
  # mainly to update the webhook certificates.
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions", "customresourcedefinitions/status"]
    verbs: ["get", "update", "patch"]
    resourceNames:
      - pipelines.tekton.dev
      - pipelineruns.tekton.dev
      - runs.tekton.dev
      - tasks.tekton.dev
      - clustertasks.tekton.dev
      - taskruns.tekton.dev
      - pipelineresources.tekton.dev
      - conditions.tekton.dev
  # knative.dev/pkg needs list/watch permissions to set up informers for the webhook.
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions"]
    verbs: ["list", "watch"]
  - apiGroups: ["admissionregistration.k8s.io"]
    # The webhook performs a reconciliation on these two resources and continuously
    # updates configuration.
    resources: ["mutatingwebhookconfigurations", "validatingwebhookconfigurations"]
    # knative starts informers on these things, which is why we need get, list and watch.
    verbs: ["list", "watch"]
  - apiGroups: ["admissionregistration.k8s.io"]
    resources: ["mutatingwebhookconfigurations"]
    # This mutating webhook is responsible for applying defaults to tekton objects
    # as they are received.
    resourceNames: ["webhook.pipeline.tekton.dev"]
    # When there are changes to the configs or secrets, knative updates the mutatingwebhook config
    # with the updated certificates or the refreshed set of rules.
    verbs: ["get", "update", "delete"]
  - apiGroups: ["admissionregistration.k8s.io"]
    resources: ["validatingwebhookconfigurations"]
    # validation.webhook.pipeline.tekton.dev performs schema validation when you, for example, create TaskRuns.
    # config.webhook.pipeline.tekton.dev validates the logging configuration against knative's logging structure
    resourceNames: ["validation.webhook.pipeline.tekton.dev", "config.webhook.pipeline.tekton.dev"]
    # When there are changes to the configs or secrets, knative updates the validatingwebhook config
    # with the updated certificates or the refreshed set of rules.
    verbs: ["get", "update", "delete"]
  - apiGroups: ["policy"]
    resources: ["podsecuritypolicies"]
    resourceNames: ["tekton-pipelines"]
    verbs: ["use"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get"]
    # The webhook configured the namespace as the OwnerRef on various cluster-scoped resources,
    # which requires we can Get the system namespace.
    resourceNames: ["tekton-pipelines"]
  - apiGroups: [""]
    resources: ["namespaces/finalizers"]
    verbs: ["update"]
    # The webhook configured the namespace as the OwnerRef on various cluster-scoped resources,
    # which requires we can update the system namespace finalizers.
    resourceNames: ["tekton-pipelines"]

---
# Copyright 2020 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: tekton-pipelines-controller
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["list", "watch"]
  # The controller needs access to these configmaps for logging information and runtime configuration.
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
    resourceNames: ["config-logging", "config-observability", "config-artifact-bucket", "config-artifact-pvc", "feature-flags", "config-leader-election", "config-registry-cert"]
  - apiGroups: ["policy"]
    resources: ["podsecuritypolicies"]
    resourceNames: ["tekton-pipelines"]
    verbs: ["use"]
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: tekton-pipelines-webhook
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["list", "watch"]
  # The webhook needs access to these configmaps for logging information.
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
    resourceNames: ["config-logging", "config-observability", "config-leader-election", "feature-flags"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["list", "watch"]
  # The webhook daemon makes a reconciliation loop on webhook-certs. Whenever
  # the secret changes it updates the webhook configurations with the certificates
  # stored in the secret.
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "update"]
    resourceNames: ["webhook-certs"]
  - apiGroups: ["policy"]
    resources: ["podsecuritypolicies"]
    resourceNames: ["tekton-pipelines"]
    verbs: ["use"]
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: tekton-pipelines-leader-election
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
rules:
  # We uses leases for leaderelection
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "list", "create", "update", "delete", "patch", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tekton-pipelines-info
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
rules:
  # All system:authenticated users needs to have access
  # of the pipelines-info ConfigMap even if they don't
  # have access to the other resources present in the
  # installed namespace.
  - apiGroups: [""]
    resources: ["configmaps"]
    resourceNames: ["pipelines-info"]
    verbs: ["get"]

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
apiVersion: v1
kind: ServiceAccount
metadata:
  name: tekton-pipelines-controller
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: tekton-pipelines-webhook
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: tekton-pipelines-controller-cluster-access
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
subjects:
  - kind: ServiceAccount
    name: tekton-pipelines-controller
    namespace: tekton-pipelines
roleRef:
  kind: ClusterRole
  name: tekton-pipelines-controller-cluster-access
  apiGroup: rbac.authorization.k8s.io
---
# If this ClusterRoleBinding is replaced with a RoleBinding
# then the ClusterRole would be namespaced. The access described by
# the tekton-pipelines-controller-tenant-access ClusterRole would
# be scoped to individual tenant namespaces.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: tekton-pipelines-controller-tenant-access
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
subjects:
  - kind: ServiceAccount
    name: tekton-pipelines-controller
    namespace: tekton-pipelines
roleRef:
  kind: ClusterRole
  name: tekton-pipelines-controller-tenant-access
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: tekton-pipelines-webhook-cluster-access
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
subjects:
  - kind: ServiceAccount
    name: tekton-pipelines-webhook
    namespace: tekton-pipelines
roleRef:
  kind: ClusterRole
  name: tekton-pipelines-webhook-cluster-access
  apiGroup: rbac.authorization.k8s.io

---
# Copyright 2020 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tekton-pipelines-controller
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
subjects:
  - kind: ServiceAccount
    name: tekton-pipelines-controller
    namespace: tekton-pipelines
roleRef:
  kind: Role
  name: tekton-pipelines-controller
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tekton-pipelines-webhook
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
subjects:
  - kind: ServiceAccount
    name: tekton-pipelines-webhook
    namespace: tekton-pipelines
roleRef:
  kind: Role
  name: tekton-pipelines-webhook
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tekton-pipelines-controller-leaderelection
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
subjects:
  - kind: ServiceAccount
    name: tekton-pipelines-controller
    namespace: tekton-pipelines
roleRef:
  kind: Role
  name: tekton-pipelines-leader-election
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tekton-pipelines-webhook-leaderelection
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
subjects:
  - kind: ServiceAccount
    name: tekton-pipelines-webhook
    namespace: tekton-pipelines
roleRef:
  kind: Role
  name: tekton-pipelines-leader-election
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tekton-pipelines-info
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
subjects:
  # Giving all system:authenticated users the access of the
  # ConfigMap which contains version information.
  - kind: Group
    name: system:authenticated
    apiGroup: rbac.authorization.k8s.io
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: tekton-pipelines-info

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: clustertasks.tekton.dev
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
    version: "v0.33.2"
spec:
  group: tekton.dev
  preserveUnknownFields: false
  versions:
    - name: v1alpha1
      served: true
      storage: false
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
    - name: v1beta1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
  names:
    kind: ClusterTask
    plural: clustertasks
    categories:
      - tekton
      - tekton-pipelines
  scope: Cluster
  conversion:
    strategy: Webhook
    webhook:
      conversionReviewVersions: ["v1beta1"]
      clientConfig:
        service:
          name: tekton-pipelines-webhook
          namespace: tekton-pipelines

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: conditions.tekton.dev
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
    version: "v0.33.2"
spec:
  group: tekton.dev
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
  names:
    kind: Condition
    plural: conditions
    categories:
      - tekton
      - tekton-pipelines
  scope: Namespaced

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: pipelines.tekton.dev
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
    version: "v0.33.2"
spec:
  group: tekton.dev
  preserveUnknownFields: false
  versions:
    - name: v1alpha1
      served: true
      storage: false
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
    - name: v1beta1
      served: true
      storage: true
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
  names:
    kind: Pipeline
    plural: pipelines
    categories:
      - tekton
      - tekton-pipelines
  scope: Namespaced
  conversion:
    strategy: Webhook
    webhook:
      conversionReviewVersions: ["v1beta1"]
      clientConfig:
        service:
          name: tekton-pipelines-webhook
          namespace: tekton-pipelines

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: pipelineruns.tekton.dev
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
    version: "v0.33.2"
spec:
  group: tekton.dev
  preserveUnknownFields: false
  versions:
    - name: v1alpha1
      served: true
      storage: false
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
        - name: Succeeded
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].status"
        - name: Reason
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].reason"
        - name: StartTime
          type: date
          jsonPath: .status.startTime
        - name: CompletionTime
          type: date
          jsonPath: .status.completionTime
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
    - name: v1beta1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
        - name: Succeeded
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].status"
        - name: Reason
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].reason"
        - name: StartTime
          type: date
          jsonPath: .status.startTime
        - name: CompletionTime
          type: date
          jsonPath: .status.completionTime
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
  names:
    kind: PipelineRun
    plural: pipelineruns
    categories:
      - tekton
      - tekton-pipelines
    shortNames:
      - pr
      - prs
  scope: Namespaced
  conversion:
    strategy: Webhook
    webhook:
      conversionReviewVersions: ["v1beta1"]
      clientConfig:
        service:
          name: tekton-pipelines-webhook
          namespace: tekton-pipelines

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: pipelineresources.tekton.dev
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
    version: "v0.33.2"
spec:
  group: tekton.dev
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
  names:
    kind: PipelineResource
    plural: pipelineresources
    categories:
      - tekton
      - tekton-pipelines
  scope: Namespaced

---
# Copyright 2020 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: runs.tekton.dev
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
    version: "v0.33.2"
spec:
  group: tekton.dev
  preserveUnknownFields: false
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
        - name: Succeeded
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].status"
        - name: Reason
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].reason"
        - name: StartTime
          type: date
          jsonPath: .status.startTime
        - name: CompletionTime
          type: date
          jsonPath: .status.completionTime
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
  names:
    kind: Run
    plural: runs
    categories:
      - tekton
      - tekton-pipelines
  scope: Namespaced

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tasks.tekton.dev
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
    version: "v0.33.2"
spec:
  group: tekton.dev
  preserveUnknownFields: false
  versions:
    - name: v1alpha1
      served: true
      storage: false
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
    - name: v1beta1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
  names:
    kind: Task
    plural: tasks
    categories:
      - tekton
      - tekton-pipelines
  scope: Namespaced
  conversion:
    strategy: Webhook
    webhook:
      conversionReviewVersions: ["v1beta1"]
      clientConfig:
        service:
          name: tekton-pipelines-webhook
          namespace: tekton-pipelines

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: taskruns.tekton.dev
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
    version: "v0.33.2"
spec:
  group: tekton.dev
  preserveUnknownFields: false
  versions:
    - name: v1alpha1
      served: true
      storage: false
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
        - name: Succeeded
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].status"
        - name: Reason
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].reason"
        - name: StartTime
          type: date
          jsonPath: .status.startTime
        - name: CompletionTime
          type: date
          jsonPath: .status.completionTime
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
    - name: v1beta1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          # One can use x-kubernetes-preserve-unknown-fields: true
          # at the root of the schema (and inside any properties, additionalProperties)
          # to get the traditional CRD behaviour that nothing is pruned, despite
          # setting spec.preserveUnknownProperties: false.
          #
          # See https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
          # See issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
        - name: Succeeded
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].status"
        - name: Reason
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Succeeded\")].reason"
        - name: StartTime
          type: date
          jsonPath: .status.startTime
        - name: CompletionTime
          type: date
          jsonPath: .status.completionTime
      # Opt into the status subresource so metadata.generation
      # starts to increment
      subresources:
        status: {}
  names:
    kind: TaskRun
    plural: taskruns
    categories:
      - tekton
      - tekton-pipelines
    shortNames:
      - tr
      - trs
  scope: Namespaced
  conversion:
    strategy: Webhook
    webhook:
      conversionReviewVersions: ["v1beta1"]
      clientConfig:
        service:
          name: tekton-pipelines-webhook
          namespace: tekton-pipelines

---
# Copyright 2020 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: Secret
metadata:
  name: webhook-certs
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
# The data is populated at install time.
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: validation.webhook.pipeline.tekton.dev
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
webhooks:
  - admissionReviewVersions: ["v1"]
    clientConfig:
      service:
        name: tekton-pipelines-webhook
        namespace: tekton-pipelines
    failurePolicy: Fail
    sideEffects: None
    name: validation.webhook.pipeline.tekton.dev
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: webhook.pipeline.tekton.dev
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
webhooks:
  - admissionReviewVersions: ["v1"]
    clientConfig:
      service:
        name: tekton-pipelines-webhook
        namespace: tekton-pipelines
    failurePolicy: Fail
    sideEffects: None
    name: webhook.pipeline.tekton.dev
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: config.webhook.pipeline.tekton.dev
  labels:
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    pipeline.tekton.dev/release: "v0.33.2"
webhooks:
  - admissionReviewVersions: ["v1"]
    clientConfig:
      service:
        name: tekton-pipelines-webhook
        namespace: tekton-pipelines
    failurePolicy: Fail
    sideEffects: None
    name: config.webhook.pipeline.tekton.dev
    objectSelector:
      matchLabels:
        app.kubernetes.io/part-of: tekton-pipelines

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: tekton-aggregate-edit
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    rbac.authorization.k8s.io/aggregate-to-edit: "true"
    rbac.authorization.k8s.io/aggregate-to-admin: "true"
rules:
  - apiGroups:
      - tekton.dev
    resources:
      - tasks
      - taskruns
      - pipelines
      - pipelineruns
      - pipelineresources
      - conditions
    verbs:
      - create
      - delete
      - deletecollection
      - get
      - list
      - patch
      - update
      - watch

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: tekton-aggregate-view
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
    rbac.authorization.k8s.io/aggregate-to-view: "true"
rules:
  - apiGroups:
      - tekton.dev
    resources:
      - tasks
      - taskruns
      - pipelines
      - pipelineruns
      - pipelineresources
      - conditions
    verbs:
      - get
      - list
      - watch

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: ConfigMap
metadata:
  name: config-artifact-bucket
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
# data:
#  # location of the gcs bucket to be used for artifact storage
#  location: "gs://bucket-name"
#  # name of the secret that will contain the credentials for the service account
#  # with access to the bucket
#  bucket.service.account.secret.name:
#  # The key in the secret with the required service account json
#  bucket.service.account.secret.key:
#  # The field name that should be used for the service account
#  # Valid values: GOOGLE_APPLICATION_CREDENTIALS, BOTO_CONFIG.
#  bucket.service.account.field.name: GOOGLE_APPLICATION_CREDENTIALS

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: ConfigMap
metadata:
  name: config-artifact-pvc
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
# data:
#   # size of the PVC volume
#   size: 5Gi
#
#   # storage class of the PVC volume
#   storageClassName: storage-class-name

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: ConfigMap
metadata:
  name: config-defaults
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
data:
  _example: |
    ################################
    #                              #
    #    EXAMPLE CONFIGURATION     #
    #                              #
    ################################

    # This block is not actually functional configuration,
    # but serves to illustrate the available configuration
    # options and document them in a way that is accessible
    # to users that "kubectl edit" this config map.
    #
    # These sample configuration options may be copied out of
    # this example block and unindented to be in the data block
    # to actually change the configuration.

    # default-timeout-minutes contains the default number of
    # minutes to use for TaskRun and PipelineRun, if none is specified.
    default-timeout-minutes: "60"  # 60 minutes

    # default-service-account contains the default service account name
    # to use for TaskRun and PipelineRun, if none is specified.
    default-service-account: "default"

    # default-managed-by-label-value contains the default value given to the
    # "app.kubernetes.io/managed-by" label applied to all Pods created for
    # TaskRuns. If a user's requested TaskRun specifies another value for this
    # label, the user's request supercedes.
    default-managed-by-label-value: "tekton-pipelines"

    # default-pod-template contains the default pod template to use
    # TaskRun and PipelineRun, if none is specified. If a pod template
    # is specified, the default pod template is ignored.
    # default-pod-template:

    # default-cloud-events-sink contains the default CloudEvents sink to be
    # used for TaskRun and PipelineRun, when no sink is specified.
    # Note that right now it is still not possible to set a PipelineRun or
    # TaskRun specific sink, so the default is the only option available.
    # If no sink is specified, no CloudEvent is generated
    # default-cloud-events-sink:

    # default-task-run-workspace-binding contains the default workspace
    # configuration provided for any Workspaces that a Task declares
    # but that a TaskRun does not explicitly provide.
    # default-task-run-workspace-binding: |
    #   emptyDir: {}

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: ConfigMap
metadata:
  name: feature-flags
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
data:
  # Setting this flag to "true" will prevent Tekton to create an
  # Affinity Assistant for every TaskRun sharing a PVC workspace
  #
  # The default behaviour is for Tekton to create Affinity Assistants
  #
  # See more in the workspace documentation about Affinity Assistant
  # https://github.com/tektoncd/pipeline/blob/main/docs/workspaces.md#affinity-assistant-and-specifying-workspace-order-in-a-pipeline
  # or https://github.com/tektoncd/pipeline/pull/2630 for more info.
  disable-affinity-assistant: "false"
  # Setting this flag to "true" will prevent Tekton scanning attached
  # service accounts and injecting any credentials it finds into your
  # Steps.
  #
  # The default behaviour currently is for Tekton to search service
  # accounts for secrets matching a specified format and automatically
  # mount those into your Steps.
  #
  # Note: setting this to "true" will prevent PipelineResources from
  # working.
  #
  # See https://github.com/tektoncd/pipeline/issues/2791 for more
  # info.
  disable-creds-init: "false"
  # This option should be set to false when Pipelines is running in a
  # cluster that does not use injected sidecars such as Istio. Setting
  # it to false should decrease the time it takes for a TaskRun to start
  # running. For clusters that use injected sidecars, setting this
  # option to false can lead to unexpected behavior.
  #
  # See https://github.com/tektoncd/pipeline/issues/2080 for more info.
  running-in-environment-with-injected-sidecars: "true"
  # Setting this flag to "true" will require that any Git SSH Secret
  # offered to Tekton must have known_hosts included.
  #
  # See https://github.com/tektoncd/pipeline/issues/2981 for more
  # info.
  require-git-ssh-secret-known-hosts: "false"
  # Setting this flag to "true" enables the use of Tekton OCI bundle.
  # This is an experimental feature and thus should still be considered
  # an alpha feature.
  enable-tekton-oci-bundles: "false"
  # Setting this flag to "true" enables the use of custom tasks from
  # within pipelines.
  # This is an experimental feature and thus should still be considered
  # an alpha feature.
  enable-custom-tasks: "false"
  # Setting this flag will determine which gated features are enabled.
  # Acceptable values are "stable" or "alpha".
  enable-api-fields: "stable"
  # Setting this flag to "false" scopes when expressions to guard a Task and
  # its dependent Tasks. This flag defaults to "true"; when expressions guard
  # the Task only. See TEP-0059 and Pipeline documentation for more details.
  scope-when-expressions-to-task: "true"

---
# Copyright 2021 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: ConfigMap
metadata:
  name: pipelines-info
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
data:
  # Contains pipelines version which can be queried by external
  # tools such as CLI. Elevated permissions are already given to
  # this ConfigMap such that even if we don't have access to
  # other resources in the namespace we still can have access to
  # this ConfigMap.
  version: "v0.33.2"

---
# Copyright 2020 Tekton Authors LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: ConfigMap
metadata:
  name: config-leader-election
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
data:
  # An inactive but valid configuration follows; see example.
  lease-duration: "60s"
  renew-deadline: "40s"
  retry-period: "10s"

---
# Copyright 2019 Tekton Authors LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: ConfigMap
metadata:
  name: config-logging
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
data:
  # Common configuration for all knative codebase
  zap-logger-config: |
    {
      "level": "info",
      "development": false,
      "sampling": {
        "initial": 100,
        "thereafter": 100
      },
      "outputPaths": ["stdout"],
      "errorOutputPaths": ["stderr"],
      "encoding": "json",
      "encoderConfig": {
        "timeKey": "ts",
        "levelKey": "level",
        "nameKey": "logger",
        "callerKey": "caller",
        "messageKey": "msg",
        "stacktraceKey": "stacktrace",
        "lineEnding": "",
        "levelEncoder": "",
        "timeEncoder": "iso8601",
        "durationEncoder": "",
        "callerEncoder": ""
      }
    }
  # Log level overrides
  loglevel.controller: "info"
  loglevel.webhook: "info"

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: ConfigMap
metadata:
  name: config-observability
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
data:
  _example: |
    ################################
    #                              #
    #    EXAMPLE CONFIGURATION     #
    #                              #
    ################################

    # This block is not actually functional configuration,
    # but serves to illustrate the available configuration
    # options and document them in a way that is accessible
    # to users that "kubectl edit" this config map.
    #
    # These sample configuration options may be copied out of
    # this example block and unindented to be in the data block
    # to actually change the configuration.

    # metrics.backend-destination field specifies the system metrics destination.
    # It supports either prometheus (the default) or stackdriver.
    # Note: Using Stackdriver will incur additional charges.
    metrics.backend-destination: prometheus

    # metrics.stackdriver-project-id field specifies the Stackdriver project ID. This
    # field is optional. When running on GCE, application default credentials will be
    # used and metrics will be sent to the cluster's project if this field is
    # not provided.
    metrics.stackdriver-project-id: "<your stackdriver project id>"

    # metrics.allow-stackdriver-custom-metrics indicates whether it is allowed
    # to send metrics to Stackdriver using "global" resource type and custom
    # metric type. Setting this flag to "true" could cause extra Stackdriver
    # charge.  If metrics.backend-destination is not Stackdriver, this is
    # ignored.
    metrics.allow-stackdriver-custom-metrics: "false"
    metrics.taskrun.level: "taskrun"
    metrics.taskrun.duration-type: "histogram"
    metrics.pipelinerun.level: "pipelinerun"
    metrics.pipelinerun.duration-type: "histogram"

---
# Copyright 2020 Tekton Authors LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: v1
kind: ConfigMap
metadata:
  name: config-registry-cert
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines
# data:
#  # Registry's self-signed certificate
#  cert: |

---
# Copyright 2019 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apps/v1
kind: Deployment
metadata:
  name: tekton-pipelines-controller
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/name: controller
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/version: "v0.33.2"
    app.kubernetes.io/part-of: tekton-pipelines
    # tekton.dev/release value replaced with inputs.params.versionTag in pipeline/tekton/publish.yaml
    pipeline.tekton.dev/release: "v0.33.2"
    # labels below are related to istio and should not be used for resource lookup
    version: "v0.33.2"
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: controller
      app.kubernetes.io/component: controller
      app.kubernetes.io/instance: default
      app.kubernetes.io/part-of: tekton-pipelines
  template:
    metadata:
      labels:
        app.kubernetes.io/name: controller
        app.kubernetes.io/component: controller
        app.kubernetes.io/instance: default
        app.kubernetes.io/version: "v0.33.2"
        app.kubernetes.io/part-of: tekton-pipelines
        # tekton.dev/release value replaced with inputs.params.versionTag in pipeline/tekton/publish.yaml
        pipeline.tekton.dev/release: "v0.33.2"
        # labels below are related to istio and should not be used for resource lookup
        app: tekton-pipelines-controller
        version: "v0.33.2"
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/os
                    operator: NotIn
                    values:
                      - windows
                  - key: karpenter.sh/provisioner-name
                    operator: DoesNotExist
      serviceAccountName: tekton-pipelines-controller
      containers:
        - name: tekton-pipelines-controller
          image: gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/controller:v0.33.2@sha256:ce01f1f89751bc2d2465d9f09f1918dcd4302551193475bdf0d23f12d5795ce1
          args: [
            # These images are built on-demand by "ko resolve" and are replaced
            # by image references by digest.
            "-kubeconfig-writer-image", "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/kubeconfigwriter:v0.33.2@sha256:8afd562ed14955320d3cfe3bab972b909734bddff4c05244f459b0bb81538052", "-git-image", "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/git-init:v0.33.2@sha256:62eb8e87d2dccdbb82d7e9b15b72236518034c163c60def06b8c5845598ef5b2", "-entrypoint-image", "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/entrypoint:v0.33.2@sha256:3c5accc771d051c7a1c2a54b97f66322f4ec2d479afd706f49c4cd59fcb8a662", "-nop-image", "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/nop:v0.33.2@sha256:33a131bd994f1d59babcdb5679d03c3cb65c581d75ef89a8507c66f809462ed8", "-imagedigest-exporter-image", "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/imagedigestexporter:v0.33.2@sha256:f08a106da17155605cce490fdf03c8bcfe09f377d5723701602d27803a907984", "-pr-image", "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/pullrequest-init:v0.33.2@sha256:fc8cbf877c4f9985d9c9343eda0f3131977b9519127a047398d3a81e648f64fa", "-workingdirinit-image", "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/workingdirinit:v0.33.2@sha256:1e48bc317ab1ef05c58b1c9f98050789d983702e56eb823dac3f400fa268869a",
            # This is gcr.io/google.com/cloudsdktool/cloud-sdk:302.0.0-slim
            "-gsutil-image", "gcr.io/google.com/cloudsdktool/cloud-sdk@sha256:27b2c22bf259d9bc1a291e99c63791ba0c27a04d2db0a43241ba0f1f20f4067f",
            # The shell image must be root in order to create directories and copy files to PVCs.
            # gcr.io/distroless/base:debug as of February 17, 2022
            # image shall not contains tag, so it will be supported on a runtime like cri-o
            "-shell-image", "gcr.io/distroless/base@sha256:3cebc059e7e52a4f5a389aa6788ac2b582227d7953933194764ea434f4d70d64",
            # for script mode to work with windows we need a powershell image
            # pinning to nanoserver tag as of July 15 2021
            "-shell-image-win", "mcr.microsoft.com/powershell:nanoserver@sha256:b6d5ff841b78bdf2dfed7550000fd4f3437385b8fa686ec0f010be24777654d6"]
          volumeMounts:
            - name: config-logging
              mountPath: /etc/config-logging
            - name: config-registry-cert
              mountPath: /etc/config-registry-cert
          env:
            - name: SYSTEM_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            # If you are changing these names, you will also need to update
            # the controller's Role in 200-role.yaml to include the new
            # values in the "configmaps" "get" rule.
            - name: CONFIG_DEFAULTS_NAME
              value: config-defaults
            - name: CONFIG_LOGGING_NAME
              value: config-logging
            - name: CONFIG_OBSERVABILITY_NAME
              value: config-observability
            - name: CONFIG_ARTIFACT_BUCKET_NAME
              value: config-artifact-bucket
            - name: CONFIG_ARTIFACT_PVC_NAME
              value: config-artifact-pvc
            - name: CONFIG_FEATURE_FLAGS_NAME
              value: feature-flags
            - name: CONFIG_LEADERELECTION_NAME
              value: config-leader-election
            - name: SSL_CERT_FILE
              value: /etc/config-registry-cert/cert
            - name: SSL_CERT_DIR
              value: /etc/ssl/certs
            - name: METRICS_DOMAIN
              value: tekton.dev/pipeline
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - all
            # User 65532 is the distroless nonroot user ID
            runAsUser: 65532
            runAsGroup: 65532
          ports:
            - name: metrics
              containerPort: 9090
            - name: profiling
              containerPort: 8008
            - name: probes
              containerPort: 8080
          livenessProbe:
            httpGet:
              path: /health
              port: probes
              scheme: HTTP
            initialDelaySeconds: 5
            periodSeconds: 10
            timeoutSeconds: 5
          readinessProbe:
            httpGet:
              path: /readiness
              port: probes
              scheme: HTTP
            initialDelaySeconds: 5
            periodSeconds: 10
            timeoutSeconds: 5
      volumes:
        - name: config-logging
          configMap:
            name: config-logging
        - name: config-registry-cert
          configMap:
            name: config-registry-cert
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app.kubernetes.io/name: controller
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/version: "v0.33.2"
    app.kubernetes.io/part-of: tekton-pipelines
    # tekton.dev/release value replaced with inputs.params.versionTag in pipeline/tekton/publish.yaml
    pipeline.tekton.dev/release: "v0.33.2"
    # labels below are related to istio and should not be used for resource lookup
    app: tekton-pipelines-controller
    version: "v0.33.2"
  name: tekton-pipelines-controller
  namespace: tekton-pipelines
spec:
  ports:
    - name: http-metrics
      port: 9090
      protocol: TCP
      targetPort: 9090
    - name: http-profiling
      port: 8008
      targetPort: 8008
    - name: probes
      port: 8080
  selector:
    app.kubernetes.io/name: controller
    app.kubernetes.io/component: controller
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines

---
# Copyright 2020 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: tekton-pipelines-webhook
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/name: webhook
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/version: "v0.33.2"
    app.kubernetes.io/part-of: tekton-pipelines
    # tekton.dev/release value replaced with inputs.params.versionTag in pipeline/tekton/publish.yaml
    pipeline.tekton.dev/release: "v0.33.2"
    # labels below are related to istio and should not be used for resource lookup
    version: "v0.33.2"
spec:
  minReplicas: 1
  maxReplicas: 5
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: tekton-pipelines-webhook
  metrics:
    - type: Resource
      resource:
        name: cpu
        targetAverageUtilization: 100

---
# Copyright 2020 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

apiVersion: apps/v1
kind: Deployment
metadata:
  # Note: the Deployment name must be the same as the Service name specified in
  # config/400-webhook-service.yaml. If you change this name, you must also
  # change the value of WEBHOOK_SERVICE_NAME below.
  name: tekton-pipelines-webhook
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/name: webhook
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/version: "v0.33.2"
    app.kubernetes.io/part-of: tekton-pipelines
    # tekton.dev/release value replaced with inputs.params.versionTag in pipeline/tekton/publish.yaml
    pipeline.tekton.dev/release: "v0.33.2"
    # labels below are related to istio and should not be used for resource lookup
    version: "v0.33.2"
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: webhook
      app.kubernetes.io/component: webhook
      app.kubernetes.io/instance: default
      app.kubernetes.io/part-of: tekton-pipelines
  template:
    metadata:
      labels:
        app.kubernetes.io/name: webhook
        app.kubernetes.io/component: webhook
        app.kubernetes.io/instance: default
        app.kubernetes.io/version: "v0.33.2"
        app.kubernetes.io/part-of: tekton-pipelines
        # tekton.dev/release value replaced with inputs.params.versionTag in pipeline/tekton/publish.yaml
        pipeline.tekton.dev/release: "v0.33.2"
        # labels below are related to istio and should not be used for resource lookup
        app: tekton-pipelines-webhook
        version: "v0.33.2"
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/os
                    operator: NotIn
                    values:
                      - windows
                  - key: karpenter.sh/provisioner-name
                    operator: DoesNotExist
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - podAffinityTerm:
                labelSelector:
                  matchLabels:
                    app.kubernetes.io/name: webhook
                    app.kubernetes.io/component: webhook
                    app.kubernetes.io/instance: default
                    app.kubernetes.io/part-of: tekton-pipelines
                topologyKey: kubernetes.io/hostname
              weight: 100
      serviceAccountName: tekton-pipelines-webhook
      containers:
        - name: webhook
          # This is the Go import path for the binary that is containerized
          # and substituted here.
          image: gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/webhook:v0.33.2@sha256:558b4c734c1dc7be8b2f3681a105bd23cc704fbf7525f0a5e7673beed40a7ca6
          # Resource request required for autoscaler to take any action for a metric
          resources:
            requests:
              cpu: 100m
              memory: 100Mi
            limits:
              cpu: 500m
              memory: 500Mi
          env:
            - name: SYSTEM_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            # If you are changing these names, you will also need to update
            # the webhook's Role in 200-role.yaml to include the new
            # values in the "configmaps" "get" rule.
            - name: CONFIG_LOGGING_NAME
              value: config-logging
            - name: CONFIG_OBSERVABILITY_NAME
              value: config-observability
            - name: CONFIG_LEADERELECTION_NAME
              value: config-leader-election
            - name: CONFIG_FEATURE_FLAGS_NAME
              value: feature-flags
            - name: WEBHOOK_SERVICE_NAME
              value: tekton-pipelines-webhook
            - name: WEBHOOK_SECRET_NAME
              value: webhook-certs
            - name: METRICS_DOMAIN
              value: tekton.dev/pipeline
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - all
            # User 65532 is the distroless nonroot user ID
            runAsUser: 65532
            runAsGroup: 65532
          ports:
            - name: metrics
              containerPort: 9090
            - name: profiling
              containerPort: 8008
            - name: https-webhook
              containerPort: 8443
            - name: probes
              containerPort: 8080
          livenessProbe:
            httpGet:
              path: /health
              port: probes
              scheme: HTTP
            initialDelaySeconds: 5
            periodSeconds: 10
            timeoutSeconds: 5
          readinessProbe:
            httpGet:
              path: /readiness
              port: probes
              scheme: HTTP
            initialDelaySeconds: 5
            periodSeconds: 10
            timeoutSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app.kubernetes.io/name: webhook
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/version: "v0.33.2"
    app.kubernetes.io/part-of: tekton-pipelines
    # tekton.dev/release value replaced with inputs.params.versionTag in pipeline/tekton/publish.yaml
    pipeline.tekton.dev/release: "v0.33.2"
    # labels below are related to istio and should not be used for resource lookup
    app: tekton-pipelines-webhook
    version: "v0.33.2"
  name: tekton-pipelines-webhook
  namespace: tekton-pipelines
spec:
  ports:
    # Define metrics and profiling for them to be accessible within service meshes.
    - name: http-metrics
      port: 9090
      targetPort: 9090
    - name: http-profiling
      port: 8008
      targetPort: 8008
    - name: https-webhook
      port: 443
      targetPort: 8443
    - name: probes
      port: 8080
  selector:
    app.kubernetes.io/name: webhook
    app.kubernetes.io/component: webhook
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-pipelines

---
`

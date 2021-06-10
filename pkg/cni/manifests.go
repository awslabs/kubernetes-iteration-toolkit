package cni

const (
	CNIPodSecurityPolicy = `
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: psp.flannel.unprivileged
  annotations:
    seccomp.security.alpha.kubernetes.io/allowedProfileNames: docker/default
    seccomp.security.alpha.kubernetes.io/defaultProfileName: docker/default
    apparmor.security.beta.kubernetes.io/allowedProfileNames: runtime/default
    apparmor.security.beta.kubernetes.io/defaultProfileName: runtime/default
spec:
  privileged: false
  volumes:
  - configMap
  - secret
  - emptyDir
  - hostPath
  allowedHostPaths:
  - pathPrefix: "/etc/cni/net.d"
  - pathPrefix: "/etc/kube-flannel"
  - pathPrefix: "/run/flannel"
  readOnlyRootFilesystem: false
  # Users and groups
  runAsUser:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  fsGroup:
    rule: RunAsAny
  # Privilege Escalation
  allowPrivilegeEscalation: false
  defaultAllowPrivilegeEscalation: false
  # Capabilities
  allowedCapabilities: ['NET_ADMIN', 'NET_RAW']
  defaultAddCapabilities: []
  requiredDropCapabilities: []
  # Host namespaces
  hostPID: false
  hostIPC: false
  hostNetwork: true
  hostPorts:
  - min: 0
    max: 65535
  # SELinux
  seLinux:
    # SELinux is unused in CaaSP
    rule: 'RunAsAny'  
`
	CNIServiceAccount = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: weave-net
  annotations:
    cloud.weave.works/launcher-info: |-
      {
        "original-request": {
          "url": "/k8s/v1.16/net.yaml?k8s-version=Q2xpZW50IFZlcnNpb246IHZlcnNpb24uSW5mb3tNYWpvcjoiMSIsIE1pbm9yOiIxOSIsIEdpdFZlcnNpb246InYxLjE5LjAiLCBHaXRDb21taXQ6ImUxOTk2NDE4MzM3N2QwZWMyMDUyZDFmMWZhOTMwYzRkNzU3NWJkNTAiLCBHaXRUcmVlU3RhdGU6ImNsZWFuIiwgQnVpbGREYXRlOiIyMDIwLTA4LTI2VDE0OjMwOjMzWiIsIEdvVmVyc2lvbjoiZ28xLjE1IiwgQ29tcGlsZXI6ImdjIiwgUGxhdGZvcm06ImRhcndpbi9hbWQ2NCJ9ClNlcnZlciBWZXJzaW9uOiB2ZXJzaW9uLkluZm97TWFqb3I6IjEiLCBNaW5vcjoiMTkrIiwgR2l0VmVyc2lvbjoidjEuMTkuOC1la3MtMS0xOS00IiwgR2l0Q29tbWl0OiI4MzJkZmQwOTRhMGMxYTBkZmU0MzAzNzc1ZTVlNjA0NzQ3M2JlZjZhIiwgR2l0VHJlZVN0YXRlOiJjbGVhbiIsIEJ1aWxkRGF0ZToiMjAyMS0wNS0wNVQxODowMToyOVoiLCBHb1ZlcnNpb246ImdvMS4xNS4xMSIsIENvbXBpbGVyOiJnYyIsIFBsYXRmb3JtOiJsaW51eC9hbWQ2NCJ9Cg==",
          "date": "Thu Jun 10 2021 05:01:46 GMT+0000 (UTC)"
        },
        "email-address": "support@weave.works"
      }
  labels:
    name: weave-net
  namespace: kube-system
`

	CNIClusterRole = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: weave-net
  annotations:
    cloud.weave.works/launcher-info: |-
      {
        "original-request": {
          "url": "/k8s/v1.16/net.yaml?k8s-version=Q2xpZW50IFZlcnNpb246IHZlcnNpb24uSW5mb3tNYWpvcjoiMSIsIE1pbm9yOiIxOSIsIEdpdFZlcnNpb246InYxLjE5LjAiLCBHaXRDb21taXQ6ImUxOTk2NDE4MzM3N2QwZWMyMDUyZDFmMWZhOTMwYzRkNzU3NWJkNTAiLCBHaXRUcmVlU3RhdGU6ImNsZWFuIiwgQnVpbGREYXRlOiIyMDIwLTA4LTI2VDE0OjMwOjMzWiIsIEdvVmVyc2lvbjoiZ28xLjE1IiwgQ29tcGlsZXI6ImdjIiwgUGxhdGZvcm06ImRhcndpbi9hbWQ2NCJ9ClNlcnZlciBWZXJzaW9uOiB2ZXJzaW9uLkluZm97TWFqb3I6IjEiLCBNaW5vcjoiMTkrIiwgR2l0VmVyc2lvbjoidjEuMTkuOC1la3MtMS0xOS00IiwgR2l0Q29tbWl0OiI4MzJkZmQwOTRhMGMxYTBkZmU0MzAzNzc1ZTVlNjA0NzQ3M2JlZjZhIiwgR2l0VHJlZVN0YXRlOiJjbGVhbiIsIEJ1aWxkRGF0ZToiMjAyMS0wNS0wNVQxODowMToyOVoiLCBHb1ZlcnNpb246ImdvMS4xNS4xMSIsIENvbXBpbGVyOiJnYyIsIFBsYXRmb3JtOiJsaW51eC9hbWQ2NCJ9Cg==",
          "date": "Thu Jun 10 2021 05:01:46 GMT+0000 (UTC)"
        },
        "email-address": "support@weave.works"
      }
  labels:
    name: weave-net
rules:
  - apiGroups:
      - ''
    resources:
      - pods
      - namespaces
      - nodes
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - networking.k8s.io
    resources:
      - networkpolicies
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - ''
    resources:
      - nodes/status
    verbs:
      - patch
      - update
`

	CNIClusterRoleBinding = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: weave-net
  annotations:
    cloud.weave.works/launcher-info: |-
      {
        "original-request": {
          "url": "/k8s/v1.16/net.yaml?k8s-version=Q2xpZW50IFZlcnNpb246IHZlcnNpb24uSW5mb3tNYWpvcjoiMSIsIE1pbm9yOiIxOSIsIEdpdFZlcnNpb246InYxLjE5LjAiLCBHaXRDb21taXQ6ImUxOTk2NDE4MzM3N2QwZWMyMDUyZDFmMWZhOTMwYzRkNzU3NWJkNTAiLCBHaXRUcmVlU3RhdGU6ImNsZWFuIiwgQnVpbGREYXRlOiIyMDIwLTA4LTI2VDE0OjMwOjMzWiIsIEdvVmVyc2lvbjoiZ28xLjE1IiwgQ29tcGlsZXI6ImdjIiwgUGxhdGZvcm06ImRhcndpbi9hbWQ2NCJ9ClNlcnZlciBWZXJzaW9uOiB2ZXJzaW9uLkluZm97TWFqb3I6IjEiLCBNaW5vcjoiMTkrIiwgR2l0VmVyc2lvbjoidjEuMTkuOC1la3MtMS0xOS00IiwgR2l0Q29tbWl0OiI4MzJkZmQwOTRhMGMxYTBkZmU0MzAzNzc1ZTVlNjA0NzQ3M2JlZjZhIiwgR2l0VHJlZVN0YXRlOiJjbGVhbiIsIEJ1aWxkRGF0ZToiMjAyMS0wNS0wNVQxODowMToyOVoiLCBHb1ZlcnNpb246ImdvMS4xNS4xMSIsIENvbXBpbGVyOiJnYyIsIFBsYXRmb3JtOiJsaW51eC9hbWQ2NCJ9Cg==",
          "date": "Thu Jun 10 2021 05:01:46 GMT+0000 (UTC)"
        },
        "email-address": "support@weave.works"
      }
  labels:
    name: weave-net
roleRef:
  kind: ClusterRole
  name: weave-net
  apiGroup: rbac.authorization.k8s.io
subjects:
  - kind: ServiceAccount
    name: weave-net
    namespace: kube-system
`

	CNIRole = `
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: weave-net
  annotations:
    cloud.weave.works/launcher-info: |-
      {
        "original-request": {
          "url": "/k8s/v1.16/net.yaml?k8s-version=Q2xpZW50IFZlcnNpb246IHZlcnNpb24uSW5mb3tNYWpvcjoiMSIsIE1pbm9yOiIxOSIsIEdpdFZlcnNpb246InYxLjE5LjAiLCBHaXRDb21taXQ6ImUxOTk2NDE4MzM3N2QwZWMyMDUyZDFmMWZhOTMwYzRkNzU3NWJkNTAiLCBHaXRUcmVlU3RhdGU6ImNsZWFuIiwgQnVpbGREYXRlOiIyMDIwLTA4LTI2VDE0OjMwOjMzWiIsIEdvVmVyc2lvbjoiZ28xLjE1IiwgQ29tcGlsZXI6ImdjIiwgUGxhdGZvcm06ImRhcndpbi9hbWQ2NCJ9ClNlcnZlciBWZXJzaW9uOiB2ZXJzaW9uLkluZm97TWFqb3I6IjEiLCBNaW5vcjoiMTkrIiwgR2l0VmVyc2lvbjoidjEuMTkuOC1la3MtMS0xOS00IiwgR2l0Q29tbWl0OiI4MzJkZmQwOTRhMGMxYTBkZmU0MzAzNzc1ZTVlNjA0NzQ3M2JlZjZhIiwgR2l0VHJlZVN0YXRlOiJjbGVhbiIsIEJ1aWxkRGF0ZToiMjAyMS0wNS0wNVQxODowMToyOVoiLCBHb1ZlcnNpb246ImdvMS4xNS4xMSIsIENvbXBpbGVyOiJnYyIsIFBsYXRmb3JtOiJsaW51eC9hbWQ2NCJ9Cg==",
          "date": "Thu Jun 10 2021 05:01:46 GMT+0000 (UTC)"
        },
        "email-address": "support@weave.works"
      }
  labels:
    name: weave-net
  namespace: kube-system
rules:
  - apiGroups:
      - ''
    resourceNames:
      - weave-net
    resources:
      - configmaps
    verbs:
      - get
      - update
  - apiGroups:
      - ''
    resources:
      - configmaps
    verbs:
      - create
`

	CNIRoleBinding = `
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: weave-net
  annotations:
    cloud.weave.works/launcher-info: |-
      {
        "original-request": {
          "url": "/k8s/v1.16/net.yaml?k8s-version=Q2xpZW50IFZlcnNpb246IHZlcnNpb24uSW5mb3tNYWpvcjoiMSIsIE1pbm9yOiIxOSIsIEdpdFZlcnNpb246InYxLjE5LjAiLCBHaXRDb21taXQ6ImUxOTk2NDE4MzM3N2QwZWMyMDUyZDFmMWZhOTMwYzRkNzU3NWJkNTAiLCBHaXRUcmVlU3RhdGU6ImNsZWFuIiwgQnVpbGREYXRlOiIyMDIwLTA4LTI2VDE0OjMwOjMzWiIsIEdvVmVyc2lvbjoiZ28xLjE1IiwgQ29tcGlsZXI6ImdjIiwgUGxhdGZvcm06ImRhcndpbi9hbWQ2NCJ9ClNlcnZlciBWZXJzaW9uOiB2ZXJzaW9uLkluZm97TWFqb3I6IjEiLCBNaW5vcjoiMTkrIiwgR2l0VmVyc2lvbjoidjEuMTkuOC1la3MtMS0xOS00IiwgR2l0Q29tbWl0OiI4MzJkZmQwOTRhMGMxYTBkZmU0MzAzNzc1ZTVlNjA0NzQ3M2JlZjZhIiwgR2l0VHJlZVN0YXRlOiJjbGVhbiIsIEJ1aWxkRGF0ZToiMjAyMS0wNS0wNVQxODowMToyOVoiLCBHb1ZlcnNpb246ImdvMS4xNS4xMSIsIENvbXBpbGVyOiJnYyIsIFBsYXRmb3JtOiJsaW51eC9hbWQ2NCJ9Cg==",
          "date": "Thu Jun 10 2021 05:01:46 GMT+0000 (UTC)"
        },
        "email-address": "support@weave.works"
      }
  labels:
    name: weave-net
  namespace: kube-system
roleRef:
  kind: Role
  name: weave-net
  apiGroup: rbac.authorization.k8s.io
subjects:
  - kind: ServiceAccount
    name: weave-net
    namespace: kube-system
`

	CNIDaemonSet = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: weave-net
  annotations:
    cloud.weave.works/launcher-info: |-
      {
        "original-request": {
          "url": "/k8s/v1.16/net.yaml?k8s-version=Q2xpZW50IFZlcnNpb246IHZlcnNpb24uSW5mb3tNYWpvcjoiMSIsIE1pbm9yOiIxOSIsIEdpdFZlcnNpb246InYxLjE5LjAiLCBHaXRDb21taXQ6ImUxOTk2NDE4MzM3N2QwZWMyMDUyZDFmMWZhOTMwYzRkNzU3NWJkNTAiLCBHaXRUcmVlU3RhdGU6ImNsZWFuIiwgQnVpbGREYXRlOiIyMDIwLTA4LTI2VDE0OjMwOjMzWiIsIEdvVmVyc2lvbjoiZ28xLjE1IiwgQ29tcGlsZXI6ImdjIiwgUGxhdGZvcm06ImRhcndpbi9hbWQ2NCJ9ClNlcnZlciBWZXJzaW9uOiB2ZXJzaW9uLkluZm97TWFqb3I6IjEiLCBNaW5vcjoiMTkrIiwgR2l0VmVyc2lvbjoidjEuMTkuOC1la3MtMS0xOS00IiwgR2l0Q29tbWl0OiI4MzJkZmQwOTRhMGMxYTBkZmU0MzAzNzc1ZTVlNjA0NzQ3M2JlZjZhIiwgR2l0VHJlZVN0YXRlOiJjbGVhbiIsIEJ1aWxkRGF0ZToiMjAyMS0wNS0wNVQxODowMToyOVoiLCBHb1ZlcnNpb246ImdvMS4xNS4xMSIsIENvbXBpbGVyOiJnYyIsIFBsYXRmb3JtOiJsaW51eC9hbWQ2NCJ9Cg==",
          "date": "Thu Jun 10 2021 05:01:46 GMT+0000 (UTC)"
        },
        "email-address": "support@weave.works"
      }
  labels:
    name: weave-net
  namespace: kube-system
spec:
  minReadySeconds: 5
  selector:
    matchLabels:
      name: weave-net
  template:
    metadata:
      labels:
        name: weave-net
    spec:
      containers:
        - name: weave
          command:
            - /home/weave/launch.sh
          env:
            - name: HOSTNAME
              valueFrom:
                fieldRef:
                  apiVersion: v1
                  fieldPath: spec.nodeName
            - name: INIT_CONTAINER
              value: 'true'
          image: 'docker.io/weaveworks/weave-kube:2.8.1'
          readinessProbe:
            httpGet:
              host: 127.0.0.1
              path: /status
              port: 6784
          resources:
            requests:
              cpu: 50m
              memory: 100Mi
          securityContext:
            privileged: true
          volumeMounts:
            - name: weavedb
              mountPath: /weavedb
            - name: dbus
              mountPath: /host/var/lib/dbus
            - name: machine-id
              mountPath: /host/etc/machine-id
              readOnly: true
            - name: xtables-lock
              mountPath: /run/xtables.lock
        - name: weave-npc
          env:
            - name: HOSTNAME
              valueFrom:
                fieldRef:
                  apiVersion: v1
                  fieldPath: spec.nodeName
          image: 'docker.io/weaveworks/weave-npc:2.8.1'
          resources:
            requests:
              cpu: 50m
              memory: 100Mi
          securityContext:
            privileged: true
          volumeMounts:
            - name: xtables-lock
              mountPath: /run/xtables.lock
      dnsPolicy: ClusterFirstWithHostNet
      hostNetwork: true
      initContainers:
        - name: weave-init
          command:
            - /home/weave/init.sh
          image: 'docker.io/weaveworks/weave-kube:2.8.1'
          securityContext:
            privileged: true
          volumeMounts:
            - name: cni-bin
              mountPath: /host/opt
            - name: cni-bin2
              mountPath: /host/home
            - name: cni-conf
              mountPath: /host/etc
            - name: lib-modules
              mountPath: /lib/modules
            - name: xtables-lock
              mountPath: /run/xtables.lock
      priorityClassName: system-node-critical
      restartPolicy: Always
      securityContext:
        seLinuxOptions: {}
      serviceAccountName: weave-net
      tolerations:
        - effect: NoSchedule
          operator: Exists
        - effect: NoExecute
          operator: Exists
      volumes:
        - name: weavedb
          hostPath:
            path: /var/lib/weave
        - name: cni-bin
          hostPath:
            path: /opt
        - name: cni-bin2
          hostPath:
            path: /home
        - name: cni-conf
          hostPath:
            path: /etc
        - name: dbus
          hostPath:
            path: /var/lib/dbus
        - name: lib-modules
          hostPath:
            path: /lib/modules
        - name: machine-id
          hostPath:
            path: /etc/machine-id
            type: FileOrCreate
        - name: xtables-lock
          hostPath:
            path: /run/xtables.lock
            type: FileOrCreate
  updateStrategy:
    type: RollingUpdate  
`
)

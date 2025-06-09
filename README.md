# ExternalDNS Hostfile Webhook

A webhook provider for Kubernetes ExternalDNS to simply edit a hosts file (`/etc/hosts` syntax) for use with a DNS server like CoreDNS in your Kubernetes cluster.

## 1. Example Deployment with ConfigMap (recommended)

This deployment uses a ConfigMap stored within Kubernetes itself to store the hosts file, requiring no additional storage.

### Helm `values.yaml` for ExternalDNS chart

For the `external-dns` chart from https://kubernetes-sigs.github.io/external-dns/

```yaml
registry: noop
policy: sync
provider:
  name: webhook
  webhook:
    image:
      repository: ghcr.io/lachlan2k/external-dns-hostsfile-webhook
      tag: main
      pullPolicy: Always
    securityContext:
      runAsUser: 65532
serviceAccount:
  create: true
  name: "external-dns"
```

### Role and RoleBinding

To allow the webhook to edit the ConfigMap, a Role and RoleBinding must be created.

```yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: cm-updater
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "create", "update"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: cm-updater-rb
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: cm-updater
subjects:
- kind: ServiceAccount
  name: external-dns
```

### Helm `values.yaml` for CoreDNS chart

For the `coredns` chart from https://coredns.github.io/helm

```yaml
replicaCount: 3
serviceType: LoadBalancer
servers:
- zones:
  - zone: .
  port: 53
  plugins:
  - name: errors
  - name: health
    configBlock: |-
      lameduck 5s
  - name: ready
  - name: hosts
    parameters: /mnt/hosts
  - name: log
extraVolumes:
  - name: hostsfile
    configMap:
      name: externaldns-hosts
extraVolumeMounts:
  - name: hostsfile
    mountPath: /mnt
```


## 2. Example Deployment with shared volume

This deployment uses a PersistentVolumeClaim to store the hosts file, requiring a StorageClass with RWX.

### PVC

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: hostsfile
  
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 1Mi
  volumeMode: Filesystem
```

### Helm `values.yaml` for ExternalDNS chart

For the `external-dns` chart from https://kubernetes-sigs.github.io/external-dns/

```yaml
registry: noop
policy: sync
provider:
  name: webhook
  webhook:
    image:
      repository: ghcr.io/lachlan2k/external-dns-hostsfile-webhook
      tag: main
      pullPolicy: Always
    extraVolumeMounts:
      - name: hostsfile
        mountPath: /mnt
    securityContext:
      runAsUser: 65532
extraVolumes:
  - name: hostsfile
    persistentVolumeClaim:
      claimName: hostsfile
serviceAccount:
  create: true
  name: "external-dns"
```

### Helm `values.yaml` for CoreDNS chart

For the `coredns` chart from https://coredns.github.io/helm

```yaml
replicaCount: 3
serviceType: LoadBalancer
servers:
- zones:
  - zone: .
  port: 53
  plugins:
  - name: errors
  - name: health
    configBlock: |-
      lameduck 5s
  - name: ready
  - name: hosts
    parameters: /mnt/hosts
  - name: log
extraVolumes:
  - name: hostsfile
    persistentVolumeClaim:
      claimName: hostsfile
extraVolumeMounts:
  - name: hostsfile
    mountPath: /mnt
```

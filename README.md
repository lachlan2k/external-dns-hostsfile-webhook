# ExternalDNS Hostfile Webhook

A webhook provider for Kubernetes ExternalDNS to simply edit a hosts file (`/etc/hosts` syntax) for use with a DNS server like CoreDNS.

## Example Deployment

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

# Alist Helm Chart

A file list program that supports multiple storages, powered by Gin and Solidjs.

## Introduction

This chart bootstraps alist on [Kubernetes](http://kubernetes.io) using the [Helm](https://helm.sh) package manager.

## Prerequisites

- Kubernetes cluster 1.20+
- Helm v3.2.0+

## Configure Alist Helm repo

No Helm repository has been provided. Please `git clone` the code project, and install it manually in the `helm` directory.

```bash
git clone https://github.com/AlistGo/alist.git

cd alist/helm
```

### Installing the Chart

Create the alist namespace.

```bash
kubectl create namespace alist
```

Install the helm chart into the alist namespace.

```bash
helm install alist . --namespace alist
```

### Values reference

The default values.yaml should be suitable for most basic deployments.

| Parameter                  | Description                                                  | Default                         |
|----------------------------|--------------------------------------------------------------|---------------------------------|
| `image.registry`           | Imag registry                                                | `docker.io`                     |
| `image.repository`         | Image name                                                   | `xhofe/alist`                   |
| `image.tag`                | Image tag                                                    | `latest`                        |
| `image.pullPolicy`         | Image pull policy                                            | `IfNotPresent`                  |
| `replicaCount`             | Number of scanner adapter Pods to run                        | `1`                             |
| `persistence.storageClass` | Specify the storageClass used to provision the volume        |                                 |
| `persistence.accessMode`   | The access mode of the volume                                | `ReadWriteOnce`                 |
| `persistence.size`         | The size of the volume                                       | `5Gi`                           |
| `service.type`             | Kubernetes service type                                      | `ClusterIP`                     |
| `service.loadBalancerIP`   | Kubernetes service loadBalancerIP                            |                                 |
| `service.http.port`        | Kubernetes service http port                                 | `5244`                          |
| `service.http.targetPort`  | Kubernetes service http targetPort                           | `5244`                          |
| `service.http.nodePort`    | Kubernetes service http nodePort                             | `35244`                         |
| `service.https.port`       | Kubernetes service https port                                | `5245`                          |
| `service.https.targetPort` | Kubernetes service https targetPort                          | `5245`                          |
| `service.https.nodePort`   | Kubernetes service https nodePort                            | `35245`                         |

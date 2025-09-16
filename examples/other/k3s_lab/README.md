# K3S Lab

**MUST** see `vars.tf` and set the variables accordingly. Also downloading a
Debian cloud image is required.

This creates 4 Debian 12 VMs and uses K3s to createa a Kubernetes cluster
with one *control-plane* node and 3 *worker* nodes. The K8S cluster traffic
is running through a separate bridge network that is created and uses The
`172.27.213.0/25` subnet. The IPv4 addresses are hard-coded:

  - 172.27.213.129 - host
  - 172.27.213.141 - control-plane node (master01)
  - 172.27.213.161 - node01
  - 172.27.213.162 - node02
  - 172.27.213.163 - node03

Because *zedamigo* needs to run various `ip` commands to create the *bridge*
and *tap* interfaces the user running `terraform apply` must have `sudo` permissions
to run those commands.

After creating the topology with `terraform apply` if `kubectl` is installed on
the host then the cluster can be accessed directly. Alternatively if `kubectl`
is not available on the host then login via `ssh` to the master node (`ssh user@172.27.213.141`)
and run `sudo kubectl` there (`sudo` is required when running it on the master
node since by default the K3s installation saves the config under
`/etc/rancher/k3s/k3s.yaml` and that is only accessible by the root user.


```shell
❯ kubectl --kubeconfig=.kube/config get nodes
NAME       STATUS   ROLES                  AGE   VERSION
master01   Ready    control-plane,master   53s   v1.33.4+k3s1
node01     Ready    <none>                 15s   v1.33.4+k3s1
node02     Ready    <none>                 39s   v1.33.4+k3s1
node03     Ready    <none>                 28s   v1.33.4+k3s1

❯ kubectl --kubeconfig=.kube/config apply -f example_nginx_deployment.yaml
deployment.apps/example-nginx-deployment created
service/nginx-service created

❯ kubectl --kubeconfig=.kube/config get pods
NAME                                      READY   STATUS              RESTARTS   AGE
example-nginx-deployment-96b9d695-bzf2m   0/1     ContainerCreating   0          6s
example-nginx-deployment-96b9d695-pjkbc   0/1     ContainerCreating   0          6s
```

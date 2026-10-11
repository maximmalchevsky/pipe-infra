# kubeconfig role
Fetches `/etc/kubernetes/admin.conf` from the control-plane node to local `.kube/config`.
Ensures the target directory exists and sets file permissions to 0600.
Prints verification command: `kubectl --kubeconfig .kube/config get nodes`.

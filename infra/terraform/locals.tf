locals {
  network_prefix = split("/", var.network_cidr)[1]

  # Ordered list of control-plane node names; the first one is the cluster-init node.
  control_names = [for i in range(var.control_count) : "control-${i + 1}"]
  worker_names  = [for i in range(var.worker_count) : "worker-${i + 1}"]

  ha_enabled  = var.control_count > 1
  control_vip = cidrhost(var.network_cidr, var.control_vip_hostnum)
  metallb_ip  = cidrhost(var.network_cidr, var.metallb_ip_hostnum)

  controls = {
    for i, name in local.control_names : name => {
      role      = "control"
      vm_id     = var.control_vm_id_start + i
      ip        = cidrhost(var.network_cidr, var.control_ip_start + i)
      resources = var.control_resources
    }
  }

  workers = {
    for i, name in local.worker_names : name => {
      role      = "worker"
      vm_id     = var.worker_vm_id_start + i
      ip        = cidrhost(var.network_cidr, var.worker_ip_start + i)
      resources = var.worker_resources
    }
  }

  nodes = merge(local.controls, local.workers)

  all_ips    = [for n in values(local.nodes) : n.ip]
  all_vm_ids = [for n in values(local.nodes) : n.vm_id]

  network_size = pow(2, 32 - tonumber(local.network_prefix))
  all_hostnums = concat([for i in range(var.control_count) : var.control_ip_start + i], [for i in range(var.worker_count) : var.worker_ip_start + i])
}

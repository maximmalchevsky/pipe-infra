output "nodes" {
  description = "Map of all cluster nodes with their role, IP address, and VM ID."
  value = {
    for k, v in local.nodes : k => {
      role  = v.role
      ip    = v.ip
      vm_id = v.vm_id
    }
  }

  precondition {
    condition     = length(distinct(local.all_ips)) == length(local.all_ips)
    error_message = "Control-plane and worker IP ranges overlap: adjust control_ip_start/worker_ip_start."
  }

  precondition {
    condition     = length(distinct(local.all_vm_ids)) == length(local.all_vm_ids)
    error_message = "Control-plane and worker VM ID ranges overlap: adjust control_vm_id_start/worker_vm_id_start."
  }

  precondition {
    condition     = alltrue([for h in local.all_hostnums : h >= 1 && h <= local.network_size - 2])
    error_message = "Node IPs fall outside network_cidr or hit the network/broadcast address: check *_ip_start and node counts."
  }

  precondition {
    condition     = !contains(local.all_ips, var.gateway)
    error_message = "A node IP equals the gateway: adjust *_ip_start."
  }

  precondition {
    condition     = !local.ha_enabled || !contains(local.all_ips, local.control_vip)
    error_message = "Control-plane VIP collides with a node IP: adjust control_vip_hostnum or *_ip_start."
  }

  precondition {
    condition     = !local.ha_enabled || local.control_vip != var.gateway
    error_message = "Control-plane VIP equals the gateway: adjust control_vip_hostnum."
  }
}

output "k8s_api_endpoint" {
  description = "Kubernetes API endpoint URL."
  value       = local.ha_enabled ? "https://${local.control_vip}:7443" : "https://${local.controls[local.control_names[0]].ip}:6443"
}

output "ansible_inventory" {
  description = "Ansible dynamic inventory matching the script-inventory JSON specification."
  value = {
    _meta = {
      hostvars = {
        for k, v in local.nodes : k => {
          ansible_host = v.ip
          node_ip      = v.ip
          node_role    = v.role
        }
      }
    }
    k8s_cluster = {
      children = ["k8s_masters", "k8s_workers"]
      vars = merge(
        {
          ansible_user               = var.vm_user
          ansible_python_interpreter = "/usr/bin/python3"
          cluster_name               = var.cluster_name
          cluster_domain             = var.search_domain
          k8s_api_ip                 = local.ha_enabled ? local.control_vip : local.controls[local.control_names[0]].ip
        },
        { for k, v in {
          ha_cluster_virtual_ip   = local.control_vip
          ha_cluster_virtual_port = "7443"
        } : k => v if local.ha_enabled }
      )
    }
    k8s_masters = {
      hosts = local.control_names
    }
    k8s_workers = {
      hosts = local.worker_names
    }
  }
}

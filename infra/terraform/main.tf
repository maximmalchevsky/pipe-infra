
resource "proxmox_virtual_environment_download_file" "ubuntu" {
  content_type   = "iso"
  datastore_id   = var.files_datastore
  node_name      = var.proxmox_node
  url            = var.ubuntu_image_url
  file_name      = "${var.cluster_name}-noble-server-cloudimg-amd64.img"
  overwrite      = false
  upload_timeout = var.image_download_timeout
}

resource "proxmox_virtual_environment_file" "user_data" {
  for_each     = local.nodes
  content_type = "snippets"
  datastore_id = var.files_datastore
  node_name    = var.proxmox_node

  source_raw {
    data = templatefile("${path.module}/templates/user-data.yaml.tftpl", {
      hostname = each.key
      domain   = var.search_domain
      user     = var.vm_user
      ssh_key  = trimspace(file(pathexpand(var.ssh_public_key_path)))
      timezone = var.timezone
    })
    file_name = "${var.cluster_name}-${each.key}-user-data.yaml"
  }
}

resource "proxmox_virtual_environment_file" "network_data" {
  for_each     = local.nodes
  content_type = "snippets"
  datastore_id = var.files_datastore
  node_name    = var.proxmox_node

  source_raw {
    data = templatefile("${path.module}/templates/network-config.yaml.tftpl", {
      address       = "${each.value.ip}/${local.network_prefix}"
      gateway       = var.gateway
      dns_servers   = var.dns_servers
      search_domain = var.search_domain
    })
    file_name = "${var.cluster_name}-${each.key}-network-config.yaml"
  }
}

resource "proxmox_virtual_environment_vm" "node" {
  for_each    = local.nodes
  name        = "${each.key}.${var.search_domain}"
  node_name   = var.proxmox_node
  vm_id       = each.value.vm_id
  description = "k8s ${each.value.role} node, managed by Terraform"
  tags        = [var.cluster_name, "k8s", each.value.role]

  on_boot         = true
  started         = true
  stop_on_destroy = true

  agent {
    enabled = true
    timeout = "15m"
  }

  cpu {
    cores = each.value.resources.cores
    type  = var.cpu_type
  }

  memory {
    dedicated = each.value.resources.memory_mb
  }

  scsi_hardware = "virtio-scsi-single"

  disk {
    datastore_id = var.vm_datastore
    file_id      = proxmox_virtual_environment_download_file.ubuntu.id
    interface    = "scsi0"
    size         = each.value.resources.disk_gb
    discard      = "on"
    iothread     = true
  }

  initialization {
    datastore_id         = var.vm_datastore
    user_data_file_id    = proxmox_virtual_environment_file.user_data[each.key].id
    network_data_file_id = proxmox_virtual_environment_file.network_data[each.key].id
  }

  network_device {
    bridge  = var.network_bridge
    vlan_id = var.vlan_id
  }

  operating_system {
    type = "l26"
  }

  serial_device {}
}

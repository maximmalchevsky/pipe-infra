variable "proxmox_endpoint" {
  type        = string
  description = "Proxmox VE API endpoint URL (e.g., https://192.168.1.10:8006/)."
}

variable "proxmox_api_token" {
  type        = string
  sensitive   = true
  description = "Proxmox VE API token in USER@REALM!TOKENID=UUID format."
}

variable "proxmox_insecure" {
  type        = bool
  default     = true
  description = "Whether to skip TLS certificate verification when connecting to the Proxmox VE API."
}

variable "proxmox_ssh_username" {
  type        = string
  default     = "root"
  description = "SSH username for uploading cloud-init snippet files to the Proxmox VE host."
}

variable "proxmox_ssh_private_key_path" {
  type        = string
  default     = "~/.ssh/id_ed25519"
  description = "private key for SSH to the Proxmox node, used to upload cloud-init snippets"
}

variable "proxmox_node" {
  type        = string
  description = "Target Proxmox VE node name for deploying VMs and storing files."
}

variable "vm_datastore" {
  type        = string
  default     = "local-lvm"
  description = "Proxmox storage pool / datastore for VM disks and cloud-init drives."
}

variable "files_datastore" {
  type        = string
  default     = "local"
  description = "Proxmox datastore for ISO images and cloud-init snippets (must allow 'iso' and 'snippets' content types)."
}

variable "cluster_name" {
  type        = string
  default     = "hackathon"
  description = "Name of the Kubernetes cluster used for host naming and resource tagging."

  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.cluster_name))
    error_message = "The cluster_name must match the pattern ^[a-z0-9-]+$ (lowercase alphanumeric and hyphens)."
  }
}

variable "control_count" {
  type        = number
  default     = 1
  description = "Number of Kubernetes control-plane nodes. Must be an odd number >= 1 for etcd quorum."

  validation {
    condition     = var.control_count >= 1 && var.control_count % 2 == 1
    error_message = "The control_count must be at least 1 and an odd number (1, 3, 5, etc.) to ensure etcd quorum."
  }
}

variable "worker_count" {
  type        = number
  default     = 2
  description = "Number of Kubernetes worker nodes. Must be >= 0."

  validation {
    condition     = var.worker_count >= 0
    error_message = "The worker_count must be greater than or equal to 0."
  }
}

variable "control_resources" {
  type = object({
    cores     = number
    memory_mb = number
    disk_gb   = number
  })
  default = {
    cores     = 2
    memory_mb = 4096
    disk_gb   = 30
  }
  description = "Resource allocation for Kubernetes control-plane nodes (CPU cores, RAM in MB, disk size in GB)."

  validation {
    condition     = var.control_resources.cores >= 1 && var.control_resources.memory_mb >= 2048 && var.control_resources.disk_gb >= 20
    error_message = "Control-plane resources must have at least 1 core, 2048 MB memory, and 20 GB disk."
  }
}

variable "worker_resources" {
  type = object({
    cores     = number
    memory_mb = number
    disk_gb   = number
  })
  default = {
    cores     = 2
    memory_mb = 4096
    disk_gb   = 30
  }
  description = "Resource allocation for Kubernetes worker nodes (CPU cores, RAM in MB, disk size in GB)."

  validation {
    condition     = var.worker_resources.cores >= 1 && var.worker_resources.memory_mb >= 1024 && var.worker_resources.disk_gb >= 20
    error_message = "Worker resources must have at least 1 core, 1024 MB memory, and 20 GB disk."
  }
}

variable "control_vm_id_start" {
  type        = number
  default     = 9101
  description = "Starting VM ID for Kubernetes control-plane nodes."
}

variable "worker_vm_id_start" {
  type        = number
  default     = 9121
  description = "Starting VM ID for Kubernetes worker nodes."
}

variable "network_cidr" {
  type        = string
  default     = "192.168.1.0/24"
  description = "Subnet CIDR for the cluster VM network."

  validation {
    condition     = can(cidrhost(var.network_cidr, 0))
    error_message = "The network_cidr must be a valid CIDR notation (e.g., 192.168.1.0/24)."
  }
}

variable "control_ip_start" {
  type        = number
  default     = 101
  description = "Starting host IP number inside network_cidr for Kubernetes control-plane nodes."
}

variable "worker_ip_start" {
  type        = number
  default     = 121
  description = "Starting host IP number inside network_cidr for Kubernetes worker nodes."
}

variable "control_vip_hostnum" {
  type        = number
  default     = 170
  description = "Host number inside network_cidr for the control-plane virtual IP (keepalived), used only when control_count > 1."
}

variable "gateway" {
  type        = string
  description = "Default gateway IP address for cluster VMs."
}

variable "dns_servers" {
  type        = list(string)
  default     = ["1.1.1.1", "8.8.8.8"]
  description = "List of DNS server IP addresses for cluster VMs."
}

variable "search_domain" {
  type        = string
  default     = "hackathon.lan"
  description = "DNS search domain for cluster VMs."
}

variable "network_bridge" {
  type        = string
  default     = "vmbr0"
  description = "Proxmox Linux bridge interface to attach cluster VM NICs to."
}

variable "vlan_id" {
  type        = number
  default     = null
  nullable    = true
  description = "Optional VLAN tag ID for the VM network interface."
}

variable "vm_user" {
  type        = string
  default     = "ubuntu"
  description = "Default sudo user created via cloud-init in the VMs."
}

variable "ssh_public_key_path" {
  type        = string
  default     = "~/.ssh/id_ed25519.pub"
  description = "Local path to the SSH public key injected into the VM default user."
}

variable "timezone" {
  type        = string
  default     = "Europe/Moscow"
  description = "Timezone configured in the VMs via cloud-init."
}

variable "cpu_type" {
  type        = string
  default     = "x86-64-v2-AES"
  description = "QEMU CPU model emulation type for cluster VMs."
}

variable "ubuntu_image_url" {
  type        = string
  default     = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
  description = "URL of the Ubuntu 24.04 cloud image to download onto Proxmox."
}

variable "image_download_timeout" {
  type        = number
  default     = 3600
  description = "Seconds Proxmox may spend downloading the cloud image (slow links need more than the provider default of 600)."

  validation {
    condition     = var.image_download_timeout >= 600
    error_message = "The image_download_timeout must be greater than or equal to 600 seconds."
  }
}


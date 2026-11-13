variable "subscription_id" {
  type = string
}

variable "tenant_id" {
  type = string
}

variable "location" {
  type    = string
  default = "spaincentral"
}

variable "admin_cidr" {
  type    = string
  default = "0.0.0.0/0"
}

variable "admin_user" {
  type    = string
  default = "azureuser"
}

variable "ssh_public_keys" {
  type = list(string)
}

# Cible B4s_v2 ; repli Standard_B2s_v2 si le crédit Azure manque.
variable "vm_size" {
  type    = string
  default = "Standard_B4s_v2"
}

variable "dns_label" {
  type = string
}

variable "longhorn_disk_size_gb" {
  type    = number
  default = 20
}

variable "platform_versions" {
  type = map(string)
  default = {
    calico         = "v3.31.7"
    helm           = "v3.22.1"
    traefik_chart  = "42.1.3"
    cert_manager   = "v1.21.2"
    metrics_server = "3.14.0"
    longhorn       = "1.12.1"
  }
  description = "Versions de la plateforme, transmises à Ansible via l'inventaire."
}

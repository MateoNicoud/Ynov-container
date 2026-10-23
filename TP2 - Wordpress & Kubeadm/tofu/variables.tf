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

variable "vm_size" {
  type    = string
  default = "Standard_B2s_v2"
}

variable "dns_label" {
  type = string
}

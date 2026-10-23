locals {
  nodes = {
    "k8s-1" = { ip = "10.0.1.11", ssh_port = 2201 }
    "k8s-2" = { ip = "10.0.1.12", ssh_port = 2202 }
    "k8s-3" = { ip = "10.0.1.13", ssh_port = 2203 }
  }
  lb_rules = {
    http  = { frontend = 80, backend = 30080, probe = "nodeport" }
    https = { frontend = 443, backend = 30443, probe = "nodeport" }
    api   = { frontend = 6443, backend = 6443, probe = "api" }
  }
}

resource "azurerm_resource_group" "tp2" {
  name     = "rg-tp2-k8s"
  location = var.location
}

resource "azurerm_virtual_network" "tp2" {
  name                = "vnet-tp2"
  location            = azurerm_resource_group.tp2.location
  resource_group_name = azurerm_resource_group.tp2.name
  address_space       = ["10.0.0.0/16"]
}

resource "azurerm_subnet" "nodes" {
  name                 = "snet-nodes"
  resource_group_name  = azurerm_resource_group.tp2.name
  virtual_network_name = azurerm_virtual_network.tp2.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_network_security_group" "nodes" {
  name                = "nsg-nodes"
  location            = azurerm_resource_group.tp2.location
  resource_group_name = azurerm_resource_group.tp2.name

  security_rule {
    name                       = "web"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_ranges    = ["30080", "30443"]
    source_address_prefix      = "Internet"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "admin"
    priority                   = 110
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_ranges    = ["22", "6443"]
    source_address_prefix      = var.admin_cidr
    destination_address_prefix = "*"
  }
}

resource "azurerm_subnet_network_security_group_association" "nodes" {
  subnet_id                 = azurerm_subnet.nodes.id
  network_security_group_id = azurerm_network_security_group.nodes.id
}

resource "azurerm_public_ip" "lb" {
  name                = "pip-tp2"
  location            = azurerm_resource_group.tp2.location
  resource_group_name = azurerm_resource_group.tp2.name
  allocation_method   = "Static"
  sku                 = "Standard"
  domain_name_label   = var.dns_label
}

resource "azurerm_lb" "tp2" {
  name                = "lb-tp2"
  location            = azurerm_resource_group.tp2.location
  resource_group_name = azurerm_resource_group.tp2.name
  sku                 = "Standard"

  frontend_ip_configuration {
    name                 = "public"
    public_ip_address_id = azurerm_public_ip.lb.id
  }
}

resource "azurerm_lb_backend_address_pool" "nodes" {
  name            = "nodes"
  loadbalancer_id = azurerm_lb.tp2.id
}

resource "azurerm_lb_probe" "nodeport" {
  name            = "nodeport"
  loadbalancer_id = azurerm_lb.tp2.id
  protocol        = "Tcp"
  port            = 30080
}

resource "azurerm_lb_probe" "api" {
  name            = "api"
  loadbalancer_id = azurerm_lb.tp2.id
  protocol        = "Tcp"
  port            = 6443
}

resource "azurerm_lb_rule" "rules" {
  for_each                       = local.lb_rules
  name                           = each.key
  loadbalancer_id                = azurerm_lb.tp2.id
  protocol                       = "Tcp"
  frontend_port                  = each.value.frontend
  backend_port                   = each.value.backend
  frontend_ip_configuration_name = "public"
  backend_address_pool_ids       = [azurerm_lb_backend_address_pool.nodes.id]
  probe_id                       = each.value.probe == "api" ? azurerm_lb_probe.api.id : azurerm_lb_probe.nodeport.id
  disable_outbound_snat          = true
}

resource "azurerm_lb_outbound_rule" "snat" {
  name                     = "snat"
  loadbalancer_id          = azurerm_lb.tp2.id
  protocol                 = "All"
  backend_address_pool_id  = azurerm_lb_backend_address_pool.nodes.id
  allocated_outbound_ports = 10000

  frontend_ip_configuration {
    name = "public"
  }
}

resource "azurerm_lb_nat_rule" "ssh" {
  for_each                       = local.nodes
  name                           = "ssh-${each.key}"
  resource_group_name            = azurerm_resource_group.tp2.name
  loadbalancer_id                = azurerm_lb.tp2.id
  protocol                       = "Tcp"
  frontend_port                  = each.value.ssh_port
  backend_port                   = 22
  frontend_ip_configuration_name = "public"
}

resource "azurerm_network_interface" "nodes" {
  for_each            = local.nodes
  name                = "nic-${each.key}"
  location            = azurerm_resource_group.tp2.location
  resource_group_name = azurerm_resource_group.tp2.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.nodes.id
    private_ip_address_allocation = "Static"
    private_ip_address            = each.value.ip
  }
}

resource "azurerm_network_interface_backend_address_pool_association" "nodes" {
  for_each                = local.nodes
  network_interface_id    = azurerm_network_interface.nodes[each.key].id
  ip_configuration_name   = "internal"
  backend_address_pool_id = azurerm_lb_backend_address_pool.nodes.id
}

resource "azurerm_network_interface_nat_rule_association" "ssh" {
  for_each              = local.nodes
  network_interface_id  = azurerm_network_interface.nodes[each.key].id
  ip_configuration_name = "internal"
  nat_rule_id           = azurerm_lb_nat_rule.ssh[each.key].id
}

resource "azurerm_linux_virtual_machine" "nodes" {
  for_each                        = local.nodes
  name                            = each.key
  location                        = azurerm_resource_group.tp2.location
  resource_group_name             = azurerm_resource_group.tp2.name
  size                            = var.vm_size
  admin_username                  = var.admin_user
  disable_password_authentication = true
  network_interface_ids           = [azurerm_network_interface.nodes[each.key].id]

  identity {
    type = "SystemAssigned"
  }

  dynamic "admin_ssh_key" {
    for_each = var.ssh_public_keys
    content {
      username   = var.admin_user
      public_key = admin_ssh_key.value
    }
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "StandardSSD_LRS"
    disk_size_gb         = 40
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "ubuntu-24_04-lts"
    sku       = "server"
    version   = "latest"
  }
}

resource "azurerm_managed_disk" "longhorn" {
  for_each             = local.nodes
  name                 = "disk-longhorn-${each.key}"
  location             = azurerm_resource_group.tp2.location
  resource_group_name  = azurerm_resource_group.tp2.name
  storage_account_type = "StandardSSD_LRS"
  create_option        = "Empty"
  disk_size_gb         = 20
}

resource "azurerm_virtual_machine_data_disk_attachment" "longhorn" {
  for_each           = local.nodes
  managed_disk_id    = azurerm_managed_disk.longhorn[each.key].id
  virtual_machine_id = azurerm_linux_virtual_machine.nodes[each.key].id
  lun                = 0
  caching            = "None"
}

resource "azurerm_role_assignment" "self_stop" {
  for_each             = local.nodes
  scope                = azurerm_linux_virtual_machine.nodes[each.key].id
  role_definition_name = "Virtual Machine Contributor"
  principal_id         = azurerm_linux_virtual_machine.nodes[each.key].identity[0].principal_id
}

resource "local_file" "inventory" {
  filename        = "${path.module}/../automatisation/inventory.yml"
  file_permission = "0644"
  content = yamlencode({
    all = {
      vars = {
        ansible_user = var.admin_user
        lb_ip        = azurerm_public_ip.lb.ip_address
        lb_fqdn      = azurerm_public_ip.lb.fqdn
      }
      children = {
        k8s_nodes = {
          children = {
            k8s_first  = { hosts = { for k in ["k8s-1"] : k => {} } }
            k8s_others = { hosts = { for k in ["k8s-2", "k8s-3"] : k => {} } }
          }
          hosts = {
            for k, v in local.nodes : k => {
              ansible_host = azurerm_public_ip.lb.ip_address
              ansible_port = v.ssh_port
              private_ip   = v.ip
            }
          }
        }
      }
    }
  })
}

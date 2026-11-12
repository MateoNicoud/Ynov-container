output "public_ip" {
  value = azurerm_public_ip.lb.ip_address
}

output "fqdn" {
  value = azurerm_public_ip.lb.fqdn
}

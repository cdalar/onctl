terraform {
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.0"
    }
  }
}

provider "hcloud" {
  # token = var.hcloud_token  # Set HCLOUD_TOKEN environment variable
}

resource "hcloud_server" "test-server" {
  name        = "test-server"
  server_type = "cx22"
  image       = "ubuntu-22.04"
  location    = "fsn1"

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }
}

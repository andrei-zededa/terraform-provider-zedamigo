resource "zedamigo_cloud_init_iso" "CI_DATA_01" {
  name = "CI_DATA_01"
  # Read long config data from a file. We could also use the `templatefile`
  # TF function if we need to set some variable values.
  user_data = file("./cloud-init/user-data")
  # Multi-line string with `heredoc`.
  meta_data      = <<-EOT
  instance-id: ${var.VM_NAME} 
  local-hostname: ${var.VM_NAME} 
  
  # Optional metadata fields
  # instance-type: t2.micro
  # availability-zone: us-east-1a
  # region: us-east-1
  # public-keys:
  #   - ssh-rsa YOUR_PUBLIC_KEY_HERE
  # public-hostname: ec2-X-X-X-X.compute-1.amazonaws.com
  # public-ipv4: X.X.X.X
  # local-ipv4: 10.X.X.X
  # placement: /placement/availability-zone
  # security-groups: default
  # ami-launch-index: 0
  # ami-manifest-path: (root) ami-name/manifest.xml
  # block-device-mapping:
  #   ami: sda1
  #   root: /dev/sda1
  EOT
  network_config = file("./cloud-init/network-config")
}

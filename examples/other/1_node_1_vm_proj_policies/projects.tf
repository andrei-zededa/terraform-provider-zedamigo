locals {
  # Names of the network-instances *as declared in the network-policy*. These
  # are only name **prefixes**: when a device joins the project Zedcloud creates
  # one network-instance per `net_instance_config` entry and names it
  # `<name>.<edge-node name>`, e.g. `ni_local_nat_kt1.ENODE_TEST_AAAA_kt1`.
  # Therefore the names below still have to be unique across the enterprise,
  # hence the `config_suffix`.
  NI_LOCAL_NAT_NAME   = "ni_local_nat_${var.config_suffix}"
  NI_SWITCH_ETH1_NAME = "ni_switch_eth1_${var.config_suffix}"
  NI_SWITCH_ETH2_NAME = "ni_switch_eth2_${var.config_suffix}"

  # Tags set on the network-instances created by the network-policy. The
  # app-policy does *not* reference network-instances by name (the name is only
  # known after the per-device suffix has been appended), it matches them by
  # tag: for every app interface Zedcloud looks up, on the device onto which the
  # app-instance is being deployed, the network-instance whose tags contain the
  # interface `netinsttag` map.
  #
  # NOTE: Zedcloud enforces a minimum length of 3 characters for both tag keys
  # and tag values.
  NI_TAG_LOCAL_NAT   = { ni_role = "local_nat" }
  NI_TAG_SWITCH_ETH1 = { ni_role = "switch_eth1" }
  NI_TAG_SWITCH_ETH2 = { ni_role = "switch_eth2" }
}

resource "zedcloud_project" "PROJECT" {
  name        = "PROJECT_TEST_${var.config_suffix}"
  title       = "PROJECT_TEST_${var.config_suffix}"
  description = <<-EOF
   A test project which auto-deploys network-instances and an edge-app-instance
   onto every edge-node that joins it, via a network-policy and an app-policy.
  EOF

  edgeview_policy {
    name  = "PROJECT_TEST_${var.config_suffix}.edgeviewPolicy"
    title = "EDGE_VIEW_POL_01"
    type  = "POLICY_TYPE_EDGEVIEW"

    edgeview_policy {
      max_expire_sec = 604800
      max_inst       = 3
      edgeview_allow = true

      edgeviewcfg {
        ext_policy {
          allow_ext = true
        }
        app_policy {
          allow_app = true
        }
        dev_policy {
          allow_dev = true
        }

        jwt_info {
          allow_sec  = 18000
          disp_url   = "${var.ZEDEDA_CLOUD_URL}/api/v1/edge-view"
          encrypt    = false
          expire_sec = "0"
          num_inst   = 1
        }
      }
    }
  }

  #### The network-policy. Every `net_instance_config` entry here is a template
  #### for a network-instance which Zedcloud creates automatically on each
  #### edge-node which joins this project. It replaces the explicit
  #### `zedcloud_network_instance` resources of the `1_node_1_vm` example.
  ####
  #### The created network-instances are *not* managed by Terraform, they only
  #### exist as long as the edge-node is a member of the project: removing the
  #### edge-node from the project (or deleting it) makes Zedcloud delete them
  #### again.
  ####
  #### The outer block is the generic "policy" wrapper (name/title/type) and the
  #### inner `network_policy` block is the policy payload. `type` must match the
  #### payload, otherwise Zedcloud rejects the policy with a
  #### "mismatch in actual policy type and policy type in config" error.
  network_policy {
    name  = "PROJECT_TEST_${var.config_suffix}.networkPolicy"
    title = "NETWORK_POL_01"
    type  = "POLICY_TYPE_NETWORK"

    network_policy {
      #### The local (NAT) network-instance on the uplink port. Identical to
      #### `zedcloud_network_instance.NET_INSTANCES_APP_NAT` of the
      #### `1_node_1_vm` example, see that file for the detailed comments about
      #### the explicit subnet and the metadata-server static route.
      net_instance_config {
        name  = local.NI_LOCAL_NAT_NAME
        title = "Auto-created local NAT network-instance"
        kind  = "NETWORK_INSTANCE_KIND_LOCAL"
        type  = "NETWORK_INSTANCE_DHCP_TYPE_V4"

        port = "uplink"
        # Makes this the default network-instance of the edge-node, which is
        # what the `default_net_instance = true` app interface below matches on.
        device_default = true

        # Pin an explicit subnet/gateway instead of letting EVE-OS auto-assign
        # one. We need a stable, known gateway IP (the network-instance bridge
        # IP) so that the static route below can point at it.
        ip {
          subnet  = "10.166.0.0/16"
          gateway = "10.166.0.1"
          dns     = ["10.166.0.1"]
          dhcp_range {
            start = "10.166.0.128"
            end   = "10.166.255.254"
          }
        }

        # Advertise an explicit host route for the EVE-OS metadata server
        # (169.254.169.254) to the app through this local network-instance. The
        # app is attached to several network-instances at once, each of them
        # advertising its own default route, so without this more specific /32
        # route the app might send metadata requests out of a switch
        # network-instance where nothing answers on 169.254.169.254.
        #
        # `gateway` is this network-instance's own bridge IP (and not
        # `output_port`) because that form is propagated verbatim to the app via
        # DHCP option 121 and does not depend on the uplink port having a
        # resolvable connected gateway.
        static_routes {
          prefix  = "169.254.169.254/32"
          gateway = "10.166.0.1"
        }

        tags = local.NI_TAG_LOCAL_NAT
      }

      #### A switch network-instance for each of the two app-shared ports.
      net_instance_config {
        name  = local.NI_SWITCH_ETH1_NAME
        title = "Auto-created switch network-instance (port = eth1)"
        kind  = "NETWORK_INSTANCE_KIND_SWITCH"
        type  = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"

        port = "eth1"

        tags = local.NI_TAG_SWITCH_ETH1
      }

      net_instance_config {
        name  = local.NI_SWITCH_ETH2_NAME
        title = "Auto-created switch network-instance (port = eth2)"
        kind  = "NETWORK_INSTANCE_KIND_SWITCH"
        type  = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"

        port = "eth2"

        tags = local.NI_TAG_SWITCH_ETH2
      }
    }
  }

  #### The app-policy. Every `apps` entry references an existing edge-app
  #### *definition* (by both id and name) which Zedcloud then instantiates on
  #### each edge-node which joins this project. It replaces the explicit
  #### `zedcloud_application_instance` resource of the `1_node_1_vm` example.
  ####
  #### Like the network-instances above, the resulting edge-app-instances are
  #### not managed by Terraform.
  ####
  #### The app-policy is applied *after* the network-policy of the same project,
  #### within the same auto-deployment job, so the network-instances the
  #### interfaces below match on already exist by the time they are resolved.
  app_policy {
    name  = "PROJECT_TEST_${var.config_suffix}.appPolicy"
    title = "APP_POL_01"
    type  = "POLICY_TYPE_APP"

    app_policy {
      apps {
        # Both are required: Zedcloud validates the policy by looking the
        # edge-app definition up by `app_id`, and the auto-deployment then looks
        # it up again by `name`.
        app_id = zedcloud_application.UBUNTU_VM_DEF.id
        name   = zedcloud_application.UBUNTU_VM_DEF.name
        title  = "Auto-deployed instance of ${zedcloud_application.UBUNTU_VM_DEF.name}"

        origin_type = "ORIGIN_LOCAL"

        # These have to be provided but they are purely informational: the
        # actual resources of the edge-app-instance are taken from the `manifest`
        # of the edge-app definition. Keep them in sync with it to avoid
        # confusion when looking at the policy in the Zedcloud UI.
        networks = 3
        cpus     = 2
        memory   = 2097152  #### 2GB, in kilobytes.
        storage  = 10485760 #### 10GB, in kilobytes.

        # How the name of the auto-deployed edge-app-instance is built. With
        # `APP_NAMING_SCHEME_APP_DEVICE` the result is
        # `<name_app_part>.<edge-node name>`, e.g.
        # `ubuntu_test.ENODE_TEST_AAAA_kt1`. If `name_app_part` is left empty
        # then the edge-app definition name is used instead. The other schemes
        # are `APP_NAMING_SCHEME_DEVICE` (just the edge-node name),
        # `APP_NAMING_SCHEME_PROJECT_DEVICE` and
        # `APP_NAMING_SCHEME_PROJECT_APP_DEVICE`.
        naming_scheme = "APP_NAMING_SCHEME_APP_DEVICE"
        name_app_part = "ubuntu_test"

        # The interfaces are matched against the interfaces of the edge-app
        # definition manifest by `intfname`, so those names must be exactly the
        # ones used there. The ACLs (including the 10022 -> 22 SSH port map) are
        # taken from the manifest as well.
        #
        # `netinstname` is required by the provider schema but it is *ignored*
        # by Zedcloud for auto-deployed instances: the actual network-instance
        # is always re-resolved per-device, either as the default one
        # (`default_net_instance = true`) or by matching `netinsttag` against the
        # network-instance tags. It is set here to the network-policy name of
        # the intended network-instance purely as documentation.
        interfaces {
          intfname  = "app_eth0"
          intforder = 1
          privateip = false

          # Use whichever network-instance is marked as the default one of the
          # edge-node, i.e. the local NAT one created by the network-policy
          # above (`device_default = true`).
          default_net_instance = true
          netinstname          = local.NI_LOCAL_NAT_NAME
        }

        interfaces {
          intfname  = "app_eth1"
          intforder = 2
          privateip = false

          netinsttag  = local.NI_TAG_SWITCH_ETH1
          netinstname = local.NI_SWITCH_ETH1_NAME
        }

        interfaces {
          intfname  = "app_eth2"
          intforder = 3
          privateip = false

          netinsttag  = local.NI_TAG_SWITCH_ETH2
          netinstname = local.NI_SWITCH_ETH2_NAME
        }
      }
    }
  }

  type = "TAG_TYPE_PROJECT"
  tag_level_settings {
    flow_log_transmission = "NETWORK_INSTANCE_FLOW_LOG_TRANSMISSION_UNSPECIFIED"
    interface_ordering    = "INTERFACE_ORDERING_ENABLED"
  }
}

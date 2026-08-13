#### The workload catalog of this example. One entry per *role*, i.e. per kind
#### of container which gets deployed, carrying the number of replicas of that
#### role and the resources which one replica gets.
####
#### Everything else in this example is generated from this map: for every
#### replica exactly one edge-app definition (`zedcloud_application`), one
#### dedicated volume-instance (`zedcloud_volume_instance`) and one
#### edge-app-instance (`zedcloud_application_instance`) is created.
####
#### Sizes:
####   - `cpus`       : vCPUs, ends up in the edge-app manifest `resources` and
####                    in the edge-app-instance `vminfo`.
####   - `memory_mb`  : RAM, ends up in the edge-app manifest `resources` where
####                    it has to be expressed in kilobytes.
####   - `persist_mb` : size of the *persistent* volume of the replica. This
####                    drive has no volume label, so Zedcloud creates an
####                    implicit blank block-storage volume of exactly this size
####                    for every edge-app-instance.
####   - `data_mb`    : size of the *dedicated* volume of the replica. This drive
####                    does have a volume label and is therefore backed by an
####                    explicit `zedcloud_volume_instance`.
####
#### `env` is the environment of the container, the equivalent of the `--env`
#### flags of a `docker run`. It is rendered into a cloud-init `runcmd` list and
#### attached to the edge-app definition as its custom config, see `edge_apps.tf`.
#### Only what is needed for the container to start is set; roles whose image
#### comes up with no configuration at all have an empty `env`.
####
#### `persist_path` / `data_path` are where the two volumes are mounted inside
#### the container. Except for PostgreSQL these are deliberately paths which no
#### image uses (`/mnt/...`): a volume mount hides whatever the image ships at
#### that path, and mounting an empty volume over e.g.
#### `/usr/share/elasticsearch/config` or `/usr/share/kibana/config` stops the
#### container from starting at all. The volumes are here to occupy the storage
#### budget; see the README for what it takes to make a container actually write
#### to one.
####
#### The totals which this catalog is supposed to add up to are checked by the
#### `resource_budget` check block at the bottom of this file.

#### The PostgreSQL superuser password. Generated rather than hardcoded, and
#### printed by the `POSTGRES_CREDENTIALS` output.
resource "random_password" "postgres_superuser" {
  length  = 16
  special = false
}

locals {
  WORKLOAD_ROLES = {
    kafka_broker = {
      replicas     = 3
      image        = "kafka"
      cpus         = 2
      memory_mb    = 4096
      persist_mb   = 512
      persist_path = "/mnt/persist"
      data_mb      = 3072
      data_path    = "/mnt/data"
      app_port     = 9092
      # This image comes up with no configuration at all.
      env = {}
    }

    kafka_controller = {
      replicas     = 3
      image        = "kafka"
      cpus         = 1
      memory_mb    = 1024
      persist_mb   = 256
      persist_path = "/mnt/persist"
      data_mb      = 256
      data_path    = "/mnt/data"
      app_port     = 9093
      # This image comes up with no configuration at all.
      env = {}
    }

    elasticsearch_master = {
      replicas     = 3
      image        = "elasticsearch"
      cpus         = 1
      memory_mb    = 2048
      persist_mb   = 256
      persist_path = "/mnt/persist"
      data_mb      = 256
      data_path    = "/mnt/data"
      app_port     = 9200
      # `discovery.type` keeps the node out of cluster formation, which also puts
      # Elasticsearch in development mode so that its bootstrap checks (notably
      # the `vm.max_map_count` one, which needs a sysctl on the host) are
      # skipped. Turning off xpack security avoids the first-boot certificate
      # and password bootstrapping. The JVM heap is added per replica further
      # down, since it is derived from `memory_mb`.
      env = {
        "discovery.type"         = "single-node"
        "xpack.security.enabled" = "false"
      }
    }

    elasticsearch_data = {
      replicas     = 3
      image        = "elasticsearch"
      cpus         = 2
      memory_mb    = 6144
      persist_mb   = 512
      persist_path = "/mnt/persist"
      data_mb      = 2560
      data_path    = "/mnt/data"
      app_port     = 9200
      # `discovery.type` keeps the node out of cluster formation, which also puts
      # Elasticsearch in development mode so that its bootstrap checks (notably
      # the `vm.max_map_count` one, which needs a sysctl on the host) are
      # skipped. Turning off xpack security avoids the first-boot certificate
      # and password bootstrapping. The JVM heap is added per replica further
      # down, since it is derived from `memory_mb`.
      env = {
        "discovery.type"         = "single-node"
        "xpack.security.enabled" = "false"
      }
    }

    elasticsearch_ingest = {
      replicas     = 1
      image        = "elasticsearch"
      cpus         = 2
      memory_mb    = 4096
      persist_mb   = 256
      persist_path = "/mnt/persist"
      data_mb      = 512
      data_path    = "/mnt/data"
      app_port     = 9200
      # `discovery.type` keeps the node out of cluster formation, which also puts
      # Elasticsearch in development mode so that its bootstrap checks (notably
      # the `vm.max_map_count` one, which needs a sysctl on the host) are
      # skipped. Turning off xpack security avoids the first-boot certificate
      # and password bootstrapping. The JVM heap is added per replica further
      # down, since it is derived from `memory_mb`.
      env = {
        "discovery.type"         = "single-node"
        "xpack.security.enabled" = "false"
      }
    }

    kibana = {
      replicas     = 2
      image        = "kibana"
      cpus         = 2
      memory_mb    = 2048
      persist_mb   = 256
      persist_path = "/mnt/persist"
      data_mb      = 128
      data_path    = "/mnt/data"
      app_port     = 5601
      # This image comes up with no configuration at all.
      env = {}
    }

    redis_cache = {
      replicas     = 4
      image        = "redis"
      cpus         = 1
      memory_mb    = 1024
      persist_mb   = 256
      persist_path = "/mnt/persist"
      data_mb      = 512
      data_path    = "/mnt/data"
      app_port     = 6379
      # This image comes up with no configuration at all.
      env = {}
    }

    redis_sentinel = {
      replicas     = 2
      image        = "redis"
      cpus         = 1
      memory_mb    = 512
      persist_mb   = 128
      persist_path = "/mnt/persist"
      data_mb      = 128
      data_path    = "/mnt/data"
      app_port     = 26379
      # This image comes up with no configuration at all.
      env = {}
    }

    postgres_primary = {
      replicas     = 2
      image        = "postgres"
      cpus         = 2
      memory_mb    = 3072
      persist_mb   = 1024
      persist_path = "/mnt/persist"
      data_mb      = 2048
      data_path    = "/var/lib/postgresql/data"
      app_port     = 5432
      # `POSTGRES_PASSWORD` is what the image refuses to start without.
      # `PGDATA` points at a *subdirectory* of the dedicated volume on purpose:
      # initdb refuses to run in a directory which is not empty, and a freshly
      # formatted volume already contains `lost+found`. The entrypoint of this
      # image starts as root, so it can create and chown that subdirectory.
      env = {
        POSTGRES_PASSWORD = random_password.postgres_superuser.result
        POSTGRES_DB       = "appdb"
        PGDATA            = "/var/lib/postgresql/data/pgdata"
      }
    }

    postgres_replica = {
      replicas     = 4
      image        = "postgres"
      cpus         = 1
      memory_mb    = 768
      persist_mb   = 384
      persist_path = "/mnt/persist"
      data_mb      = 2048
      data_path    = "/var/lib/postgresql/data"
      app_port     = 5432
      # `POSTGRES_PASSWORD` is what the image refuses to start without.
      # `PGDATA` points at a *subdirectory* of the dedicated volume on purpose:
      # initdb refuses to run in a directory which is not empty, and a freshly
      # formatted volume already contains `lost+found`. The entrypoint of this
      # image starts as root, so it can create and chown that subdirectory.
      env = {
        POSTGRES_PASSWORD = random_password.postgres_superuser.result
        POSTGRES_DB       = "appdb"
        PGDATA            = "/var/lib/postgresql/data/pgdata"
      }
    }
  }
}

locals {
  #### Expand the roles into one flat map with one entry per replica, keyed
  #### `<role>_<replica no.>`, e.g. `kafka_broker_1`. That key is used as the
  #### `for_each` key of all the per-replica resources, so it also ends up in
  #### the names of the Zedcloud objects.
  WORKLOADS_WITHOUT_PORTS = merge([
    for role, spec in local.WORKLOAD_ROLES : {
      for replica in range(1, spec.replicas + 1) :
      "${role}_${replica}" => merge(spec, {
        role    = role
        replica = replica
      })
    }
  ]...)

  #### Every edge-app-instance gets its own inbound port on the edge-node, which
  #### is port-mapped to the service port (`app_port`) of the container - the
  #### equivalent of `docker run -p <node_port>:<app_port>`. The ports are handed
  #### out in the (stable) alphabetical order of the workload names so that they
  #### don't shift around when unrelated entries of the catalog are edited.
  ####
  #### `zedamigo` forwards host port `<ssh_port> + 2` to edge-node port 10080,
  #### so the first workload of the list is the one which is reachable from the
  #### host without any extra port forwarding, see the `EDGE_APP_LOCALHOST_PORT`
  #### output.
  WORKLOAD_NODE_PORT_BASE = 10080

  WORKLOADS = {
    for idx, name in sort(keys(local.WORKLOADS_WITHOUT_PORTS)) :
    name => merge(local.WORKLOADS_WITHOUT_PORTS[name], {
      node_port = local.WORKLOAD_NODE_PORT_BASE + idx

      #### The only part of the container environment which is not static per
      #### role: Elasticsearch sizes its JVM heap from `ES_JAVA_OPTS`, and the
      #### usual rule of thumb is half of the RAM of the node. Without it the JVM
      #### picks a heap from the *edge-node* RAM (256GB) rather than from the
      #### limit of the app-instance and gets OOM-killed.
      env = merge(
        local.WORKLOADS_WITHOUT_PORTS[name].env,
        local.WORKLOADS_WITHOUT_PORTS[name].image == "elasticsearch"
        ? { ES_JAVA_OPTS = "-Xms${floor(local.WORKLOADS_WITHOUT_PORTS[name].memory_mb / 2)}m -Xmx${floor(local.WORKLOADS_WITHOUT_PORTS[name].memory_mb / 2)}m" }
        : {},
      )
    })
  }

  #### The container environment of each workload, rendered as the cloud-init
  #### user-data which Zedcloud attaches to the edge-app definition.
  ####
  #### For a container app-instance EVE-OS does not run cloud-init: it parses the
  #### user-data and turns every `KEY=VALUE` entry of the cloud-config `runcmd`
  #### list into an environment variable of the container. So this is how one
  #### expresses `docker run --env KEY=VALUE`.
  ####
  #### `yamlencode` is used rather than hand-written YAML so that values which
  #### need quoting (the `ES_JAVA_OPTS` ones contain spaces) are quoted correctly.
  WORKLOAD_CLOUD_INIT = {
    for name, w in local.WORKLOADS :
    name => "#cloud-config\n${yamlencode({
      runcmd = [for k in sort(keys(w.env)) : "${k}=${w.env[k]}"]
    })}"
  }

  RESOURCE_BUDGET = {
    app_instances = length(local.WORKLOADS)
    vcpus         = sum([for w in local.WORKLOADS : w.cpus])
    memory_mb     = sum([for w in local.WORKLOADS : w.memory_mb])
    persist_mb    = sum([for w in local.WORKLOADS : w.persist_mb])
    dedicated_mb  = sum([for w in local.WORKLOADS : w.data_mb])
  }
}

#### The whole point of this example is to put a specific, known amount of load
#### onto a single edge-node, so guard the totals. A `check` block only produces
#### a warning, it does not stop an apply, which is what we want here: editing
#### the catalog on purpose should be easy, doing it by accident should be loud.
check "resource_budget" {
  assert {
    condition = (local.RESOURCE_BUDGET.app_instances == 27 &&
      local.RESOURCE_BUDGET.vcpus == 38 &&
      local.RESOURCE_BUDGET.memory_mb == 61 * 1024 &&
      local.RESOURCE_BUDGET.persist_mb == 10 * 1024 &&
    local.RESOURCE_BUDGET.dedicated_mb == 33 * 1024)

    error_message = <<-EOF
     `local.WORKLOAD_ROLES` no longer adds up to the resource budget of this
     example (27 app-instances, 38 vCPU, 61 GB RAM, 10 GB persistent volumes,
     33 GB dedicated volumes). It currently adds up to:
       app-instances      = ${local.RESOURCE_BUDGET.app_instances}
       vCPU               = ${local.RESOURCE_BUDGET.vcpus}
       memory             = ${local.RESOURCE_BUDGET.memory_mb} MB
       persistent volumes = ${local.RESOURCE_BUDGET.persist_mb} MB
       dedicated volumes  = ${local.RESOURCE_BUDGET.dedicated_mb} MB
     EOF
  }
}

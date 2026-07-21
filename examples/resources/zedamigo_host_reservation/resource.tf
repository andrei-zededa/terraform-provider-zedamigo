# The reservations tree must be pre-created on the target by the operator; the
# provider only claims among existing (empty) slot files and never invents
# capacity. For a host offering 8 CPUs, 16 GB and one spare disk for reservation:
#
#   sudo install -d -m 0755 \
#     /var/lib/zedamigo/reservations/cpus/unit \
#     /var/lib/zedamigo/reservations/ram/gb \
#     /var/lib/zedamigo/reservations/devs/dev
#   for i in $(seq 0 7);  do sudo touch /var/lib/zedamigo/reservations/cpus/unit/$i; done
#   for i in $(seq 0 15); do sudo touch /var/lib/zedamigo/reservations/ram/gb/$i;    done
#   sudo touch /var/lib/zedamigo/reservations/devs/dev/sdb

resource "zedamigo_host_reservation" "example" {
  # Root directory holding the capacity slot files. Optional; defaults to
  # /var/lib/zedamigo/reservations.
  # path = "/var/lib/zedamigo/reservations"

  cpus = 4
  mem  = 8 # GB
  devs = ["/dev/sdb"]
}

# The computed *_reserved attributes report exactly what was claimed — e.g. the
# specific host CPU core IDs, which can be fed into an edge node's cpu_pins.
output "reserved_cpu_cores" {
  value = zedamigo_host_reservation.example.cpus_reserved
}

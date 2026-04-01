#!/bin/sh
# qemu-wrapper: bind-mounted over /usr/libexec/qemu-kvm inside the virt-launcher
# container. libvirt calls this for both QEMU capability probes and domain launches.
#
# Notes:
#   - virtio-net-pci-non-transitional matches the other non-transitional devices
#   - The libvirt-injected -sandbox seccomp filter blocks SLIRP socket syscalls
#     so it is stripped from the argument list before exec.
#   - hostfwd must bind 0.0.0.0 explicitly; the default is 127.0.0.1 which is
#     not reachable by Podman's slirp4netns port-forwarding proxy.

# Log all invocations when debug mode is enabled (PODVIRT_DEBUG=1).
LOG=/var/cache/libvirt/qemu/capabilities/qemu-wrapper.log
if [ "${PODVIRT_DEBUG:-0}" = "1" ]; then
  echo "--- $(date -u +%T) args: $*" >> "$LOG" 2>/dev/null
fi

for arg; do
  case "$arg" in
    guest=*)
      if [ "${PODVIRT_DEBUG:-0}" = "1" ]; then
        echo "--- $(date -u +%T) VM LAUNCH: injecting netdev for $arg" >> "$LOG" 2>/dev/null
      fi

      # Strip -sandbox and its argument.
      set -- $(printf '%s\n' "$@" | \
        awk '/^-sandbox$/{skip=2} skip>0{skip--; next} {print}')

      # Inject qcow2 format nodes for any qcow2 disk files.
      # virt-launcher generates a raw file-driver blockdev for every HostDisk:
      #   -blockdev {"driver":"file","filename":"...qcow2","node-name":"libvirt-N-storage",...}
      #   -device   {"driver":"virtio-blk...",            "drive":"libvirt-N-storage",...}
      qcow2_nodes=$(printf '%s\n' "$@" | awk '
        /^-blockdev$/ { nxt=1; next }
        nxt && /"driver":"file"/ && /\.qcow2"/ {
          match($0, /"node-name":"[^"]+"/)
          print substr($0, RSTART+13, RLENGTH-14)
        }
        { nxt=0 }
      ')
      if [ -n "$qcow2_nodes" ]; then
        if [ "${PODVIRT_DEBUG:-0}" = "1" ]; then
          echo "--- $(date -u +%T) qcow2 format-node injection for: $qcow2_nodes" >> "$LOG" 2>/dev/null
        fi
        set -- $(printf '%s\n' "$@" | awk -v nodes="$qcow2_nodes" '
          BEGIN {
            n = split(nodes, arr, "\n")
            for (i=1; i<=n; i++) if (arr[i]!="") qmap[arr[i]] = 1
            nxt = 0
          }
          /^-blockdev$/ { nxt=1; print; next }
          nxt {
            print
            for (nd in qmap)
              if ($0 ~ ("\"node-name\":\"" nd "\"")) {
                print "-blockdev"
                print "{\"driver\":\"qcow2\",\"file\":\"" nd "\",\"node-name\":\"" nd "-fmt\"}"
              }
            nxt = 0; next
          }
          {
            line = $0
            for (nd in qmap)
              gsub("\"drive\":\"" nd "\"", "\"drive\":\"" nd "-fmt\"", line)
            print line
          }
        ')
      fi

      exec /usr/libexec/qemu-kvm-real "$@" \
        -netdev user,id=net0,hostfwd=tcp:0.0.0.0:22-:22 \
        -device virtio-net-pci-non-transitional,netdev=net0 \
        2>>/var/cache/libvirt/qemu/capabilities/qemu-kvm-real.stderr.log
      ;;
  esac
done
exec /usr/libexec/qemu-kvm-real "$@"

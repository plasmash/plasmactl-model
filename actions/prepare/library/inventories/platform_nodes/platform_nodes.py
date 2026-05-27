"""
v2 dumb yaml aggregator for platform inventory.

Reads:
  /tmp/cluster                                  -> cluster name
  platforms/<cluster>/platform.yaml             -> infrastructure constants
  platforms/<cluster>/nodes/*.yaml              -> per-node state (SSoT)

Emits Ansible inventory groups derived from each node's `zones` list
(every ancestor in the dotted path becomes a group; hosts get added to
all of them). Host vars come straight from node yaml; group vars come
from platform.yaml + aggregations across nodes. No provider API calls.
"""

import glob
import ipaddress
import os
import sys

import yaml

try:
    from ansible.plugins.inventory import BaseInventoryPlugin
except ImportError:  # tests run without ansible installed
    class BaseInventoryPlugin:
        pass

DOCUMENTATION = """
    extends_documentation_fragment:
        - inventory_cache
"""

CLUSTER_FILE = "/tmp/cluster"  # patched in tests


class InventoryModule(BaseInventoryPlugin):
    NAME = "platform_nodes"

    # Public Ansible callback ------------------------------------------------

    def verify_file(self, path):
        return True

    def parse(self, inventory, loader, path, cache=True):
        super().parse(inventory, loader, path)
        self._populate(inventory, os.getcwd())

    # Internal entry point used by tests + parse() ---------------------------

    def _populate(self, inventory, cwd):
        cluster = self._read_cluster()
        platform = self._load_platform_yaml(cluster, cwd)
        nodes = self._load_nodes(cluster, cwd)

        self.groups = set()
        self._add_group(inventory, "platform")
        self._build_zone_groups(inventory, nodes)
        self._add_hosts_to_zone_groups(inventory, nodes)
        self._set_host_variables(inventory, nodes, platform)
        self._set_group_variables(inventory, nodes, platform)

    # Helpers ---------------------------------------------------------------

    def _read_cluster(self):
        try:
            with open(CLUSTER_FILE) as f:
                return f.read().strip()
        except FileNotFoundError:
            print(f"ERROR: missing {CLUSTER_FILE}", file=sys.stderr)
            sys.exit(1)

    def _load_platform_yaml(self, cluster, cwd):
        path = os.path.join(cwd, "platforms", cluster, "platform.yaml")
        try:
            with open(path) as f:
                return yaml.safe_load(f) or {}
        except FileNotFoundError:
            print(f"ERROR: {path} not found", file=sys.stderr)
            sys.exit(1)

    def _load_nodes(self, cluster, cwd):
        node_glob = os.path.join(cwd, "platforms", cluster, "nodes", "*.yaml")
        nodes = []
        for path in sorted(glob.glob(node_glob)):
            with open(path) as f:
                data = yaml.safe_load(f) or {}
            data["_filename_id"] = os.path.basename(path).rsplit(".yaml", 1)[0]
            nodes.append(data)
        return nodes

    def _add_group(self, inventory, group):
        if group not in self.groups:
            inventory.add_group(group)
            self.groups.add(group)

    def _build_zone_groups(self, inventory, nodes):
        self.platform_main_groups = []  # NEW: ordered list of top-level children of "platform"
        seen_main = set()
        for node in nodes:
            for zone in node.get("zones", []) or []:
                parts = zone.split(".")
                for i in range(1, len(parts) + 1):
                    g = ".".join(parts[:i])
                    self._add_group(inventory, g)
                    if i > 1:
                        parent = ".".join(parts[: i - 1])
                        inventory.add_child(parent, g)
                # Track 2-part groups (immediate children of "platform")
                if len(parts) >= 2 and parts[0] == "platform":
                    main = f"platform.{parts[1]}"
                    if main not in seen_main:
                        seen_main.add(main)
                        self.platform_main_groups.append(main)

    def _add_hosts_to_zone_groups(self, inventory, nodes):
        self.host_groups = {}  # NEW: track for labels emission
        for node in nodes:
            host_id = node["_filename_id"]
            inventory.add_host(host_id, "platform")
            seen = ["platform"]  # ordered for deterministic labels emission
            for zone in node.get("zones", []) or []:
                parts = zone.split(".")
                for i in range(1, len(parts) + 1):
                    g = ".".join(parts[:i])
                    if g in seen:
                        continue
                    inventory.add_host(host_id, g)
                    seen.append(g)
            self.host_groups[host_id] = seen

    def _set_host_variables(self, inventory, nodes, platform):
        features = platform.get("features") or {}
        private_vip_network = (platform.get("networking") or {}).get("private_vip_network", "")

        for node in nodes:
            host_id = node["_filename_id"]
            net = node.get("network") or {}
            public_ip = net.get("public_ip", "")
            private_ip = net.get("private_ip", "")
            failover_ip = net.get("failover_ip", "")
            raw_disks = list(node.get("disks") or [])
            # v2 ships disks as path-only strings; this dict-fallback is defensive
            # in case a future schema evolution adds the {path, type, capacity_gb} form.
            disks = [d["path"] if isinstance(d, dict) else d for d in raw_disks]
            pmeta = node.get("provider_metadata") or {}

            inventory.set_variable(host_id, "hostname", node.get("hostname") or node["_filename_id"])
            inventory.set_variable(host_id, "ansible_host", public_ip)
            inventory.set_variable(host_id, "public_ip", public_ip)
            inventory.set_variable(host_id, "private_ip", private_ip)
            inventory.set_variable(host_id, "failover_ip", failover_ip)
            inventory.set_variable(host_id, "public_mac", net.get("public_mac", ""))
            inventory.set_variable(host_id, "private_mac", net.get("private_mac", ""))
            inventory.set_variable(host_id, "host_id", pmeta.get("server_id", host_id))
            inventory.set_variable(host_id, "provider", pmeta.get("provider", ""))
            inventory.set_variable(host_id, "disks", disks)

            if private_ip:
                inventory.set_variable(host_id, "private_network", f"{private_ip}/24")
            else:
                inventory.set_variable(host_id, "private_network", "")
            inventory.set_variable(host_id, "private_vip_network", private_vip_network or "")
            if public_ip:
                # Prefer node:join enrichment values if present (provider-aware:
                # OVH writes public_gateway from /specifications/network and
                # public_prefix=32 for point-to-point routing). Fall back to
                # legacy Scaleway-pattern derivation when absent so existing
                # node yamls keep working.
                node_prefix = net.get("public_prefix") or 0
                node_gateway = net.get("public_gateway") or ""
                prefix = node_prefix if node_prefix > 0 else 24
                gateway = node_gateway if node_gateway else ".".join(public_ip.split(".")[:3]) + ".1"
                inventory.set_variable(host_id, "public_network", f"{public_ip}/{prefix}")
                inventory.set_variable(host_id, "public_gateway", gateway)
            else:
                inventory.set_variable(host_id, "public_network", "")
                inventory.set_variable(host_id, "public_gateway", "")
            if failover_ip:
                inventory.set_variable(host_id, "failover_network", f"{failover_ip}/32")
            else:
                inventory.set_variable(host_id, "failover_network", "")

            inventory.set_variable(host_id, "display_os_rebuild_confirmation",
                                   bool(features.get("display_os_rebuild_confirmation", False)))
            inventory.set_variable(host_id, "display_data_wipe_confirmation",
                                   bool(features.get("display_data_wipe_confirmation", False)))
            inventory.set_variable(host_id, "os_wipe_data",
                                   bool(features.get("os_wipe_data", False)))

            groups_for_host = self.host_groups.get(host_id, [])
            labels_str = " ".join(f"{g}=true" for g in groups_for_host)
            inventory.set_variable(host_id, "labels", labels_str)

    def _set_group_variables(self, inventory, nodes, platform):
        infra = platform.get("infrastructure") or {}
        networking = platform.get("networking") or {}
        etcd_port = str(networking.get("etcd_port", "2379"))

        # vRack VLAN ID lifted from platform.yaml.infrastructure.private_vlan_id
        vlan_id = infra.get("private_vlan_id")
        if vlan_id is None:
            print(
                "ERROR: platform.yaml is missing 'infrastructure.private_vlan_id' "
                "(required for vRack/RPN VLAN configuration on the os_flatcar template)",
                file=sys.stderr,
            )
            sys.exit(1)
        inventory.set_variable("platform", "os_flatcar_private_vlan_id", vlan_id)

        # private VIP = second usable IP of the configured network
        private_vip_network = networking.get("private_vip_network")
        if private_vip_network:
            net = ipaddress.ip_network(private_vip_network)
            inventory.set_variable("platform", "machine_private_vip", str(net[1]))
            inventory.set_variable("platform", "private_vip_network", private_vip_network)

        # control-plane aggregations
        control_nodes = [n for n in nodes if "platform.foundation.cluster.control" in (n.get("zones") or [])]
        etcd_endpoints = [
            f"https://{n['network']['private_ip']}:{etcd_port}"
            for n in control_nodes
            if (n.get('network') or {}).get('private_ip')
        ]
        etcd_hosts = [
            n['network']['private_ip']
            for n in control_nodes
            if (n.get('network') or {}).get('private_ip')
        ]
        inventory.set_variable("platform", "machine_etcd_endpoints", etcd_endpoints)
        inventory.set_variable("platform", "machine_etcd_hosts", etcd_hosts)
        inventory.set_variable("platform", "machine_etcd_port", etcd_port)

        # all hosts
        all_private_ips = [
            n['network']['private_ip']
            for n in nodes
            if (n.get('network') or {}).get('private_ip')
        ]
        inventory.set_variable("platform", "machine_hosts", all_private_ips)
        inventory.set_variable("platform", "machine_nodes_count", len(nodes))
        inventory.set_variable("platform", "machine_resources_main_groups", self.platform_main_groups)

        # storage cluster disk count = sum(disks - 1 OS disk per node)
        storage_count = 0
        for n in nodes:
            disks = n.get("disks") or []
            if disks:
                storage_count += max(0, len(disks) - 1)
        inventory.set_variable("platform", "storage_cluster_disk_count", storage_count)

        # ingress IPs
        web_node = next(
            (n for n in nodes if "platform.foundation.network.ingress.web" in (n.get("zones") or [])),
            None,
        )
        if web_node:
            inventory.set_variable(
                "platform",
                "machine_web_ingress_public_ip",
                (web_node.get("network") or {}).get("public_ip", ""),
            )
        mail_node = next(
            (n for n in nodes if "platform.foundation.network.ingress.mail" in (n.get("zones") or [])),
            None,
        )
        if mail_node:
            inventory.set_variable(
                "platform",
                "machine_mail_ingress_public_ip",
                (mail_node.get("network") or {}).get("public_ip", ""),
            )


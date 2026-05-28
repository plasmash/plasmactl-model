"""Tests for the platform_nodes inventory plugin (VLAN-id optionality).

Run from this file's directory:
    python3 -m unittest -v test_platform_nodes
"""

import importlib.util
import os
import tempfile
import unittest
from pathlib import Path

MOD_PATH = os.path.join(
    os.path.dirname(os.path.abspath(__file__)),
    "library", "inventories", "platform_nodes", "platform_nodes.py",
)

NODE_YAML = (
    "hostname: test-control\n"
    "zones:\n"
    "  - platform\n"
    "  - platform.foundation.cluster.control\n"
    "network:\n"
    "  private_ip: 192.168.0.1\n"
)


def load_module():
    spec = importlib.util.spec_from_file_location("platform_nodes", MOD_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FakeInventory:
    """Records add_*/set_variable calls so tests can assert on them."""

    def __init__(self):
        self.groups = set()
        self.children = []
        self.hosts = {}
        self.vars = {}

    def add_group(self, group):
        self.groups.add(group)

    def add_child(self, parent, child):
        self.children.append((parent, child))

    def add_host(self, host, group=None):
        self.hosts.setdefault(host, set())
        if group:
            self.hosts[host].add(group)

    def set_variable(self, entity, key, value):
        self.vars[(entity, key)] = value


class VlanIdOptionalTests(unittest.TestCase):
    def setUp(self):
        self.module = load_module()
        self._tmp = tempfile.TemporaryDirectory()
        self.root = Path(self._tmp.name)
        self.cluster = "testcluster"
        cluster_file = self.root / "cluster"
        cluster_file.write_text(self.cluster)
        # platform_nodes reads the cluster name from this constant.
        self.module.CLUSTER_FILE = str(cluster_file)

    def tearDown(self):
        self._tmp.cleanup()

    def _write_platform(self, platform_yaml):
        base = self.root / "platforms" / self.cluster
        (base / "nodes").mkdir(parents=True, exist_ok=True)
        (base / "platform.yaml").write_text(platform_yaml)
        (base / "nodes" / "test-control.yaml").write_text(NODE_YAML)

    def _run(self, platform_yaml):
        self._write_platform(platform_yaml)
        inventory = FakeInventory()
        module_obj = self.module.InventoryModule()
        module_obj._populate(inventory, str(self.root))
        return inventory

    def test_absent_vlan_id_is_optional(self):
        # No infrastructure.private_vlan_id -> plugin must NOT error and must
        # NOT set the variable (so the os_flatcar role default applies).
        inventory = self._run("networking:\n  private_network: 192.168.0.0/16\n")
        self.assertNotIn(("platform", "os_flatcar_private_vlan_id"), inventory.vars)

    def test_present_vlan_id_is_set(self):
        # When provided in platform.yaml, it must be emitted as an inventory var.
        inventory = self._run(
            "infrastructure:\n"
            "  private_vlan_id: 100\n"
            "networking:\n"
            "  private_network: 192.168.0.0/16\n"
        )
        self.assertEqual(inventory.vars[("platform", "os_flatcar_private_vlan_id")], 100)


if __name__ == "__main__":
    unittest.main()

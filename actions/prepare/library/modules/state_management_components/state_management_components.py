#!/usr/bin/python

from abc import ABC, abstractmethod
import os
import subprocess
import re
import concurrent.futures
from ansible.module_utils.basic import AnsibleModule
import sys
import logging
from datetime import datetime
import glob
import shutil
import tempfile

try:
    import json
except ImportError:
    import simplejson as json


class VersionFetcher(ABC):
    @abstractmethod
    def fetch_version(self):
        pass


class OSVersionFetcher(VersionFetcher):
    def fetch_version(self, state_machine_resource_default_tag):
        return self.get_env_vars_with_mrv(state_machine_resource_default_tag)

    def get_env_vars_with_mrv(self, state_machine_resource_default_tag):
        result = os.popen("source /etc/profile && env").read()
        lines = [line for line in result.split("\n") if "MRV" in line]
        data = {}
        for line in lines:
            key, value = line.strip().split("=")
            key = key.replace("_MRV", "").lower()
            data[key] = {state_machine_resource_default_tag: [value]}
        return data


class ClusterVersionFetcher(VersionFetcher):
    def fetch_version(self, state_machine_resource_default_tag):
        resources = [
            "statefulset",
            "sparkapplication",
            "objectbucketclaim",
            "storageclass",
            "prometheus",
            "deployment",
            "daemonset",
        ]
        data = {}
        for kind in resources:
            cmd = f"""/opt/bin/kubectl get {kind} -A -o json | jq '.items[] |
            select(.metadata.annotations.mrn and .metadata.annotations.mrv) |
            {{(.metadata.annotations.mrn | gsub("-"; "_")):
            {{ "{state_machine_resource_default_tag}" :
            [.metadata.annotations.mrv]}}}}' | jq -s 'add' """
            result = subprocess.check_output(cmd, shell=True).decode("utf-8")
            if result.rstrip() != "null":
                data.update(json.loads(result))
        return data


class EntitiesVersionFetcher(VersionFetcher):
    def fetch_version(self, state_schemas_uri, state_machine_resource_default_tag):
        data = {}
        base_url = state_schemas_uri.rstrip('/')

        try:
            list_cmd = 'curl -s -f "{}/search/artifacts?limit=1000"'.format(base_url)
            list_result = subprocess.run(
                list_cmd, shell=True, capture_output=True, text=True, timeout=30
            )

            if list_result.returncode != 0:
                logging.error("Failed to fetch Apicurio artifacts: {}".format(list_result.stderr))
                return data

            response = json.loads(list_result.stdout)
            artifacts = response.get("artifacts", [])

            entity_ids = [
                a.get("id") for a in artifacts
                if "__entities__" in a.get("id", "")
            ]
            if not entity_ids:
                return data

            tmpdir = tempfile.mkdtemp()
            try:
                cmd_parts = [
                    'curl', '-s', '-Z', '--parallel-max', '20', '--http1.1',
                ]
                for j, artifact_id in enumerate(entity_ids):
                    url = "{}/groups/default/artifacts/{}/meta".format(base_url, artifact_id)
                    outfile = os.path.join(tmpdir, "resp_{}.json".format(j))
                    cmd_parts.extend(['-o', outfile, url])

                subprocess.run(cmd_parts, capture_output=True, text=True, timeout=60)

                for j, artifact_id in enumerate(entity_ids):
                    outfile = os.path.join(tmpdir, "resp_{}.json".format(j))
                    try:
                        if os.path.exists(outfile) and os.path.getsize(outfile) > 0:
                            with open(outfile, 'r') as f:
                                meta = json.load(f)
                            if "version" in meta:
                                version = meta["version"]
                                data[artifact_id] = {
                                    state_machine_resource_default_tag: [version],
                                    "current_version": version,
                                }
                    except (json.JSONDecodeError, IOError) as e:
                        logging.warning("Failed to parse entity {}: {}".format(artifact_id, e))

            finally:
                shutil.rmtree(tmpdir, ignore_errors=True)

        except subprocess.TimeoutExpired:
            logging.error("Timeout fetching entity versions from Apicurio")
        except json.JSONDecodeError as e:
            logging.error("JSON decode error from Apicurio: {}".format(e))
        except Exception as e:
            logging.error("Error fetching entity versions: {}".format(e))

        return data


class ImagesVersionFetcher(VersionFetcher):
    def fetch_version(self, state_images_uri, state_images_auth):
        # Fetch from Docker Registry
        docker_registry_data = self.fetch_from_registry(
            state_images_uri, state_images_auth
        )

        # Fetch Images Locally
        local_images_data = self.fetch_local_images(state_images_uri)
        merged_data = {}
        for key, value in docker_registry_data.items():
            if key in local_images_data:
                local_images_data[key].update(value)
            else:
                local_images_data[key] = value
        merged_data = local_images_data
        return merged_data

    def fetch_local_images(self, state_images_uri):
        try:
            cmd = 'sudo crictl images -o json | jq "[.images[] | select(.repoTags[]? | contains(\\"{}\\")) | .repoTags[]] | unique"'.format(state_images_uri)
            result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=30)
            if result.returncode != 0:
                logging.error("crictl/jq failed: {}".format(result.stderr))
                return {}

            data = {}
            for repo_tag in json.loads(result.stdout):
                if ":" not in repo_tag:
                    continue
                image_path, tag = repo_tag.rsplit(":", 1)
                if "_" not in tag or "__" in tag:
                    continue
                tag_id, tag_suffix = tag.split("_", 1)
                tag_suffix = tag_suffix.replace("-cur", "")
                mrn = image_path.replace(state_images_uri + "/", "").replace("/", "__").replace("-", "_")
                if mrn in data:
                    data[mrn][tag_suffix] = [tag_id]
                else:
                    data[mrn] = {tag_suffix: [tag_id]}
            return data
        except subprocess.TimeoutExpired:
            logging.error("crictl timed out")
        except Exception as e:
            logging.error("Error fetching local images: {}".format(e))
        return {}

    def fetch_from_registry(self, state_images_uri, state_images_auth):
        base_url = "https://{}".format(state_images_uri)
        auth = state_images_auth

        def get_paths():
            """Fetch catalog with pagination."""
            cmd = 'curl -k -i -s -H "Authorization: Basic {}" '.format(auth)
            last = ""
            paths = []
            link_exists = True
            try:
                while link_exists:
                    catalog_url = "/v2/_catalog?n=100" if last == "" else "'" + last + "'"
                    resp = subprocess.check_output(
                        cmd + base_url + catalog_url,
                        shell=True, timeout=40
                    ).decode("utf-8").strip().split("\n")
                    values = json.loads(resp[-1])
                    if "errors" in values:
                        break
                    paths.extend(values.get("repositories", []))
                    link_exists = False
                    for line in resp:
                        if "link" in line.lower():
                            match = re.search(r"<(.*?)>", line)
                            if match:
                                last = match.group(1)
                                link_exists = True
                                break
            except Exception as e:
                logging.error("Exception fetching catalog: {}".format(e))
            return paths

        def fetch_batch(batch_paths, tmpdir):
            """Run curl -Z --http2 for a batch."""
            cmd_parts = [
                'curl', '-k', '-s',
                '--http2',
                '-Z',
                '--parallel-max', '100',
                '-H', 'Authorization: Basic {}'.format(auth),
            ]
            for j, path in enumerate(batch_paths):
                url = "{}/v2/{}/tags/list".format(base_url, path)
                outfile = os.path.join(tmpdir, "resp_{}.json".format(j))
                cmd_parts.extend(['-o', outfile, url])

            subprocess.run(cmd_parts, capture_output=True, text=True, timeout=60)

        def parse_batch(batch_paths, tmpdir):
            """Parse output files from a completed batch."""
            batch_results = {}
            for j, path in enumerate(batch_paths):
                outfile = os.path.join(tmpdir, "resp_{}.json".format(j))
                try:
                    if not os.path.exists(outfile) or os.path.getsize(outfile) == 0:
                        continue
                    with open(outfile, 'r') as f:
                        data = json.load(f)
                    if "errors" in data or "name" not in data:
                        continue
                    tags = data.get("tags") or []
                    current_tag_dict = {}
                    for tag in tags:
                        if "_" in tag and "__" not in tag:
                            prefix, suffix = tag.split("_")[0], tag.split("_")[1].replace("-cur", "")
                            if suffix in current_tag_dict:
                                # Dedup: both VERSION_default and VERSION_default-cur
                                # produce the same prefix
                                if prefix not in current_tag_dict[suffix]:
                                    current_tag_dict[suffix].append(prefix)
                            else:
                                current_tag_dict[suffix] = [prefix]
                            if tag.endswith("-cur"):
                                current_tag_dict["current_version"] = prefix
                    resource_name = data["name"].replace("/", "__").replace("-", "_")
                    batch_results[resource_name] = current_tag_dict
                except (json.JSONDecodeError, IOError) as e:
                    logging.warning("Failed to parse {}: {}".format(path, e))
            return batch_results

        paths = get_paths()

        results = {}
        batch_size = 100
        max_retries = 3
        for i in range(0, len(paths), batch_size):
            batch_paths = paths[i:i + batch_size]
            for attempt in range(1, max_retries + 1):
                tmpdir = tempfile.mkdtemp()
                try:
                    fetch_batch(batch_paths, tmpdir)
                    results.update(parse_batch(batch_paths, tmpdir))
                    break
                except subprocess.TimeoutExpired:
                    logging.error("Curl batch {} attempt {}/{} timed out".format(
                        i // batch_size, attempt, max_retries))
                except Exception as e:
                    logging.error("Curl batch {} attempt {}/{} error: {}".format(
                        i // batch_size, attempt, max_retries, e))
                finally:
                    shutil.rmtree(tmpdir, ignore_errors=True)
            else:
                logging.error("Curl batch {} failed after {} attempts, skipping".format(
                    i // batch_size, max_retries))

        return results


class StateCreator:
    def __init__(self, merged_data):
        self.merged_data = merged_data

    def state_helper(self, new_version, tag="default", old_version=None):
        if old_version is None:
            old_version = {}
        return {
            tag: {
                "mrv": old_version.get(tag, []),
                "mrv_cur": old_version.get(
                    "current_version", old_version.get(tag, None)
                ),
                "exists": bool(old_version.get(tag, [])),
                "fresh": new_version in old_version.get(tag, []),
                "build": new_version not in old_version.get(tag, []),
            }
        }

    def create_state(self, state_platform_components):
        current_states = self.merged_data
        states = {}
        for mrn, comp in state_platform_components.items():
            try:
                mrv = comp["mrv"]
                states[mrn] = comp

                for tag in comp["mrt"]:
                    mrk = comp.get("mrk", "")

                    # Helpers are local Ansible tools - no state tracking needed
                    if mrk == "helper":
                        state = {
                            tag: {
                                "mrv": [mrv],
                                "mrv_cur": mrv,
                                "exists": True,
                                "fresh": True,
                                "build": False,
                            }
                        }
                    elif mrn in current_states and tag in current_states.get(mrn, {}):
                        # Found in merged_data - compare versions
                        machine_mrsn = comp["mrsn"].replace("-", "_")
                        states[mrn][
                            "password"
                        ] = '{{ %s_service_plain_password|default("") }}' % (
                            machine_mrsn
                        )
                        current_version = current_states[mrn]
                        state = self.state_helper(
                            new_version=mrv, tag=tag, old_version=current_version
                        )
                    else:
                        # Not in merged_data - needs building
                        state = self.state_helper(tag=tag, new_version=mrv)

                    if "state" in states[mrn]:
                        states[mrn]["state"].update(state)
                    else:
                        states[mrn]["state"] = state
            except Exception as e:
                logging.error("EXCEPTION for {}: {}".format(mrn, e))
        return states


def manage_files(directory, prefix, max_files):
    files = sorted(
        glob.glob(os.path.join(directory, f"{prefix}_*.json")), key=os.path.getctime
    )
    while len(files) >= max_files:
        os.remove(files.pop(0))


def main():
    folder_path = "/tmp/state_management_logs/"
    if not os.path.exists(folder_path):
        os.mkdir(folder_path)

    prefix = "vars_with_state"
    max_files = 4

    logging.basicConfig(
        level=logging.INFO,
        filename=f"{folder_path}fetch_local_images.log",
        format="%(asctime)s:%(levelname)s:%(message)s",
        filemode="w",
    )
    logging.debug("Start")

    os_fetcher = OSVersionFetcher()
    images_fetcher = ImagesVersionFetcher()
    cluster_fetcher = ClusterVersionFetcher()

    module_args = dict(
        state_platform_components=dict(type=dict, required=True),
        state_images_uri=dict(type="str", required=True),
        state_schemas_uri=dict(type="str", required=True),
        state_machine_resource_default_tag=dict(type="str", required=True),
        state_images_auth=dict(type="str", required=True),
    )
    result = {}
    module = AnsibleModule(argument_spec=module_args, supports_check_mode=True)

    if module.check_mode:
        module.exit_json(**result)

    merged_data = {}
    states = {}
    try:
        os_data = os_fetcher.fetch_version(
            module.params["state_machine_resource_default_tag"]
        )
        with open(folder_path + "os_data", "w") as json_file:
            json.dump(os_data, json_file, indent=4)
        if os_data is not None:
            merged_data.update(os_data)

        else:
            module.warn("Warning: OS data is None. Skipping update")

        if os_data == {}:
            module.warn("Warning: OS data is empty. Skipping update")

        cluster_data = cluster_fetcher.fetch_version(
            module.params["state_machine_resource_default_tag"]
        )
        with open(folder_path + "cluster_data", "w") as json_file:
            json.dump(cluster_data, json_file, indent=4)
        if cluster_data is not None:
            merged_data.update(cluster_data)
        else:
            module.warn("Warning: Cluster data is None. Skipping update.")

        if cluster_data == {}:
            module.warn("Warning: cluster data is empty. Skipping update")

        images_data = images_fetcher.fetch_version(
            module.params["state_images_uri"],
            module.params["state_images_auth"],
        )
        with open(folder_path + "images_data", "w") as json_file:
            json.dump(images_data, json_file, indent=4)
        if images_data is not None:
            merged_data.update(images_data)

        else:
            module.warn("Warning: Images data is None. Skipping update")

        if module.params.get("state_schemas_uri"):
            entities_fetcher = EntitiesVersionFetcher()
            entities_data = entities_fetcher.fetch_version(
                module.params["state_schemas_uri"],
                module.params["state_machine_resource_default_tag"],
            )
            with open(folder_path + "entities_data", "w") as json_file:
                json.dump(entities_data, json_file, indent=4)
            if entities_data:
                merged_data.update(entities_data)
                logging.info("Fetched {} entity versions from Apicurio".format(len(entities_data)))
            else:
                module.warn("Warning: Entities data is empty")

        with open(folder_path + "merge_data", "w") as json_file:
            json.dump(merged_data, json_file, indent=4)
        if images_data == {}:
            module.warn("Warning: images data is empty. Skipping update")
        if merged_data != {}:
            state_creator = StateCreator(merged_data)
            states = state_creator.create_state(
                module.params["state_platform_components"]
            )

        # Create a new state file (atomic write)
        new_file_path = "/tmp/vars_with_state.json"

        # Backup the old vars_with_state.json to the log folder if it exists
        if os.path.exists(new_file_path):
            creation_time = os.path.getctime(new_file_path)
            timestamp = datetime.fromtimestamp(creation_time).strftime("%Y%m%d_%H%M%S")
            old_file_path = os.path.join(folder_path, f"{prefix}_{timestamp}.json")
            shutil.copy(new_file_path, old_file_path)
            manage_files(folder_path, prefix, max_files)

        # Write to temp file, then atomic rename (prevents partial state files)
        tmp_fd, tmp_path = tempfile.mkstemp(
            dir=os.path.dirname(new_file_path), suffix=".tmp"
        )
        try:
            with os.fdopen(tmp_fd, "w") as json_file:
                json.dump(states, json_file, indent=4)
            os.rename(tmp_path, new_file_path)
        except Exception:
            os.unlink(tmp_path)
            raise

        module.exit_json(**states)
    except subprocess.CalledProcessError as e:
        if merged_data != {}:
            state_creator = StateCreator(merged_data)
            states = state_creator.create_state(
                module.params["state_platform_components"]
            )
        new_file_path = "/tmp/vars_with_state.json"
        tmp_fd, tmp_path = tempfile.mkstemp(
            dir=os.path.dirname(new_file_path), suffix=".tmp"
        )
        try:
            with os.fdopen(tmp_fd, "w") as json_file:
                json.dump(states, json_file, indent=4)
            os.rename(tmp_path, new_file_path)
        except Exception:
            os.unlink(tmp_path)
            raise
        module.warn(str(e))
        module.exit_json(**states)
    except Exception as e:
        module.fail_json(msg=str(e))


if __name__ == "__main__":
    main()

#!/usr/bin/python3

import argparse
import yaml

parser = argparse.ArgumentParser(description='Gather resources from Antrea repository')
parser.add_argument('yaml_files', metavar='file', type=argparse.FileType('r'), nargs='+',
                    help='List of yaml files for processing')
parser.add_argument('--platform', choices=['kubernetes', 'openshift'], default='kubernetes')
parser.add_argument('--version', default='main')

args = parser.parse_args()

version = args.version
if version is None or version == "" or version == "main":
    version = "latest"
else:
    version = 'v' + version

platform = args.platform
if platform == "kubernetes":
    image = "antrea-ubuntu"
else:
    image = "antrea-ubi"

out = {
    'apiVersion': 'operator.antrea.vmware.com/v1',
    'kind': 'AntreaInstall',
    'metadata': {
        'name': 'antrea-install',
        'namespace': 'antrea-operator'
    },
    'spec': {
        'antreaImage': 'antrea/%s:%s' % (image, version),
        'antreaPlatform': platform
    }
}

for f in args.yaml_files:
    docs = yaml.load_all(f, Loader=yaml.Loader)

    for doc in docs:
        if doc and doc.get('kind') == 'ConfigMap' and doc.get('metadata', {}).get('name') == 'antrea-config':
            out['spec']['antreaAgentConfig'] = doc.get('data', {}).get('antrea-agent.conf', "")
            out['spec']['antreaCNIConfig'] = doc.get('data', {}).get('antrea-cni.conflist', "")
            out['spec']['antreaControllerConfig'] = doc.get('data', {}).get('antrea-controller.conf', "")

print(yaml.dump(out))

exit(0)

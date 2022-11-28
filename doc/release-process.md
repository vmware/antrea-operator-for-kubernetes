# Release Process for Antrea Operator for Kubernetes

## Overview

The following doc describes the release and certification process of the Antrea 
Operator for Kubernetes project.
The release process is combined of several steps, and each should complete 
successfully in order to release the Operator.

## Building Antrea-derived resources

Some the resources within the Antrea Operator repository are derived from the 
content of the Antrea repository, and therefore should be updated upon release 
of Antrea versions.\
These resources can be generated automatically by running a 
Makefile target:

```bash
make antrea-resources 
```

This Makefile target generates the following resources:
- `antrea-manifest/antrea.yml`: a template file containing Antrea resources and 
derived from Antrea
- `config/rbac/role.yaml`: contains all the roles which are required for the 
deployment for Antrea. As the Antrea Operator grants the roles to Antrea 
services, it should own any role which Antrea requires. Additionally, the Antrea 
Operator needs some additional roles for its operations. These roles are defined
statically in `config/rbac/role_base_ocp.yaml`.
- `config/samples/operator_v1_antreainstall.yaml`: contains installation 
configuration for the Antrea Operator, including the configuration template for
the Antrea services and the version of Antrea image for use during the 
installation.

As Antrea changes quite frequently, it is advised to update these resources 
after Antrea version is released. 

## Building deployment resources

Antrea Operator repository contains two directories with content for automated 
deployment of Antrea within Kubernetes and OpenShift: 
- deploy/kubernetes 
- deploy/openshift

Part of the content of these directories is Antrea-derived and is generated
automatically using the Makefile targets below.

Generate Kubernetes deployment bundle using:
```bash
make bundle
```

Generate OpenShift deployment bundle using:
```bash
make ocpbundle
```

**Note:** both of these targets execute `make antrea-resources` so there is no 
need to run this target.

The generated resources above should be committed into the Antrea Operator
repository before it is tagged, so the Antrea generated content will match the 
released Antrea version.  

## Tagging the Antrea Operator repository

Antrea Operator repository is tagged with the same version number that matches 
the generated content of the Antrea version.
So for Antrea v1.9.0, Antrea Operator should be tagged with v1.9.0 as well.

While
[Antrea Operator upstream repository](https://github.com/vmware/antrea-operator-for-kubernetes) 
is tagged, GitHub Action triggers a build of
the Antrea Operator image and then pushes the image into 
[Docker Hub registry](https://hub.docker.com/r/antrea/antrea-operator).

This action is triggered by `Build and push a release image` GitHub Action.

## Certifying with Red Hat OpenShift

Red Hat OpenShift certification requires two different steps: certification of 
the Antrea Operator image, and certification of the Antrea Operator bundle.

### Antrea Operator image certification

Antrea Operator image certification should run after the image is created, 
pushed and tagged. To trigger the execution of the image certification 
automation, run the
[image certification GitHub Action](https://github.com/vmware/antrea-operator-for-kubernetes/actions/workflows/certification.yml)
manually.

This action receives as parameters the image version which should be identical 
to the release tag of the Antrea version (and therefore also the Antrea Operator
version), and whether this Antrea Operator image should be tagged as "latest" 
within the Reh Hat repository.

The certification process requires a set of secrets which are predefined in the
Antrea Operator GitHub repository:
- `OCP_PROJECT_NAMESPACE`: a fixed value for all Red Hat OpenShift 
certifications, `redhat-isv-containers`
- `REGISTRY_LOGIN_USERNAME` and `REGISTRY_LOGIN_PASSWORD`: the credentials which
are used to log into the Red Hat OpenShift certification registry. Can be 
obtained at the
[Red Hat certification website](https://connect.redhat.com/manage/projects) 
within the Antrea Operator image certification project.
- `PFLT_PYXIS_API_TOKEN`: an access token to the Red Hat certification website.
Can be obtained [here](https://connect.redhat.com/account/api-keys).
- `PFLT_CERTIFICATION_PROJECT_ID`: the identification of the Antrea Operator 
certification project at the Red Hat certification website. 

Successful execution should reflect on the image being marked at "certified" in
the [Red Hat certification website](https://connect.redhat.com/manage/projects).

Once the image is certified, it should be published at the Antrea Operator 
project page within
[Red Hat certification website](https://connect.redhat.com/manage/projects). 

### Antrea Operator bundle certification

The certification of the Antrea Operator OpenShift bundle is performed using 
following procedure:

#### Branch the Red Hat operator certification repository
Red Hat maintains a
[repository](https://github.com/redhat-openshift-ecosystem/certified-operators)
which contains the bundles of all the certified OpenShift operators, and each of
their versions.

#### Create a directory for the new version of Antrea Operator
This repository already contains the previous versions of the Antrea Operator
bundle, under `operators/antrea-operator-for-kubernetes` directory.

This directory contains a separate directory for each release of the bundle.
To certify a new release, a new directory should be created with the version as
a name.

#### Generate content for the Antrea Operator bundle certification

In the Antrea Operator repository directory, run the following Makefile target:
```bash
make ocpcertification
```

which will update the content of the `bundle/manifests` and `bundle/metadata` 
directories. 

**Note:** generation should take place after the Antrea Operator repository is 
tagged and the Antrea Operator image is built. The certification process 
requires that the image will be identified by its hash (instead of just the 
version tag) and therefore should be performed after the Operator image is 
released. Hence, Antrea Operator version x cannot contain the certification 
bundle for version x.

#### Populate version directory in certification repository

Copy the `bundle/manifests` and `bundle/metadata` directories with the generated
contents from the Antrea Operator repository into the version directory in the 
OpenShift operator certification
[repository](https://github.com/redhat-openshift-ecosystem/certified-operators)
which was created above.

Commit these changes into the Red Hat repository by creating a pull request on 
GitHub. Red Hat certification requires that the PR title will follow the
following scheme:

operator antrea-operator-for-kubernetes (<version>)

Where <version> contains the same version id which is used as directory name.

Pushing this PR triggers the certification process on Red Hat CI, and results 
are posted via email and on the
[Red Hat certification website](https://connect.redhat.com/manage/projects)

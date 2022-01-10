# Openshift Knative Eventing

This repository holds Openshift's fork of
[`knative/eventing`](https://github.com/knative/eventing) with additions and
fixes needed only for the OpenShift side of things.

## Release Schedule

A little table to coordinate the midstream release/ci automation for our fork repo.

### Possible Outdate Periodics

When adding a new release, check if the corresponding version in "Possible Outdated Periodics" column is no longer supported in [OpenShift Serverless - Support Dates Matrix](https://access.redhat.com/support/policy/updates/openshift#ossrvless) and remove the corresponding periodics in such case.

| Release | Release Manager | Possible Outdated Periodics |
|---------|-----------------|-----------------------------|
| v0.25   | devguyio        | OSS 1.15 (v0.21 periodics)  |
| v0.26   | lberk           | OSS 1.16 (v0.22 periodics)  |
| v1.0    | aliok           | OSS 1.17 (v0.23 periodics)  |
| v1.1    | matzew          | OSS 1.18 (v0.24 periodics)  |
| v1.2    | pierdipi        | OSS 1.19 (v0.25 periodics)  |
| v1.3    | devguyio        | OSS 1.20 (v0.26 periodics)  |
| v1.4    | lberk           | OSS 1.21 (v1.0 periodics)   |
| v1.5    | aliok           | OSS 1.22 (v1.1 periodics)   |
| v1.6    | matzew          | OSS 1.23 (v1.2 periodics)   |
| v1.7    | pierdipi        | OSS 1.24 (v1.3 periodics)   |

## CI Rotation

| Week Starting        | CI Duty  |
|----------------------|----------|
| 2022-01-10T01:00:00Z | aliok    |
| 2022-01-17T01:00:00Z | devguyio |
| 2022-01-24T01:00:00Z | lberk    |
| 2022-01-31T01:00:00  | matzew   |
| 2022-02-07T01:00:00Z | pierDipi |
| 2022-02-14T01:00:00Z | aliok    |
| 2022-02-21T01:00:00Z | devguyio |
| 2022-02-28T01:00:00Z | lberk    |
| 2022-03-07T01:00:00  | matzew   |
| 2022-03-14T01:00:00Z | pierDipi |


## How this repository works ?

The `main` branch holds up-to-date specific [openshift files](./openshift)
that are necessary for CI setups and maintaining it. This includes:

- Scripts to create a new release branch from `upstream`
- CI setup files
  - operator configuration (for Openshift's CI setup)
  - tests scripts
- Operator's base configurations

Each release branch holds the upstream code for that release and our
openshift's specific files.

## CI Setup

For the CI setup, two repositories are of importance:

- This repository
- [openshift/release](https://github.com/openshift/release) which
  contains the configuration of CI jobs that are run on this
  repository

All of the following is based on OpenShift’s CI operator
configs. General understanding of that mechanism is assumed in the
following documentation.

The job manifests for the CI jobs are generated automatically. The
basic configuration lives in the
[/ci-operator/config/openshift/knative-eventing](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/knative-eventing) folder of the
[openshift/release](https://github.com/openshift/release) repository. These files include which version to
build against (OCP 4.0 for our recent cases), which images to build
(this includes all the images needed to run Knative and also all the
images required for running e2e tests) and which command to execute
for the CI jobs to run (more on this later).

Before we can create the ci-operator configs mentioned above, we need
to make sure there are Dockerfiles for all images that we need
(they’ll be referenced by the ci-operator config hence we need to
create them first). The [generate-dockerfiles.sh](https://github.com/openshift/knative-eventing/blob/master/openshift/ci-operator/generate-dockerfiles.sh) script takes care of
creating all the Dockerfiles needed automatically. The files now need
to be committed to the branch that CI is being setup for.

The basic ci-operator configs mentioned above are generated via the
generate-release.sh file in the openshift/knative-eventing
repository. They are generated to alleviate the burden of having to
add all possible test images to the manifest manually, which is error
prone.

Once the file is generated, it must be committed to the
[openshift/release](https://github.com/openshift/release) repository, as the other manifests linked above. The
naming schema is `openshift-knative-eventing-BRANCH.yaml`, thus the
files existing already correspond to our existing releases and the
main branch itself.

After the file has been added to the folder as mentioned above, the
job manifests itself will need to be generated as is described in the
corresponding [ci-operator documentation](https://docs.google.com/document/d/1SQ_qlkcplqhe8h6ONXdgBr7YUVbs4oRSj4ISl3gpLW4/edit#heading=h.8w7nj9363nsd).

Once all of this is done (Dockerfiles committed, ci-operator config
created and job manifests generated) a PR must be opened against
[openshift/release](https://github.com/openshift/releaseopenshift/release)
to include all the ci-operator related files. Once
this PR is merged, the CI setup for that branch is active.

## Create a new release

### Deliverables:

- Tagged images on quay.io
- An OLM manifest referencing these images
- Install documentation

### High-level steps for a release

#### Building upstream

[The release README](openshift/release/README.md) provides more detail for
steps 1. and 2. below.


1. Create a release branch from the upstream’s release tag, i.e. release-v0.5.0. This is created in the fork that we maintain of upstream.
2. Create a CI job for that branch in openshift/release. See our CI setup instructions for deeper information.
3. Do whatever you need to do to make this CI pass
4. Create a “dummy” PR with a ci file, which contains the current output of “date”. This is to trigger CI explicitly.
5. Make sure that PR is green and merge it. This will trigger the images to become available on the CI registry.

#### Mirroring images to quay.io

1. Make sure the images for the release are built and “promoted”. This can be verified by “docker pull”ing them, for example: “docker pull registry.svc.ci.openshift.org/openshift/knative-v0.5.0:knative-eventing-controller”
2. If that’s the case, create/amend an image mirroring mapping file as described here.

#### Update the operator

1. Copy the upstream release manifest to the operator’s deploy/resources directory. All files in this directory will be applied, so remove any old ones.
2. Update the manifest[s] to replace gcr.io images with quay.io images.
3. Build/test the operator
4. Commit the source
5. `export VERSION="vX.Y.Z"        # replace X.Y.Z as appropriate`
6.. `operator-sdk build --docker-build-args "--build-arg version=$VERSION" quay.io/openshift-knative/$OPERATOR_NAME:$VERSION`
7. `docker push quay.io/openshift-knative/$OPERATOR_NAME:$VERSION`
8. `git tag $VERSION; git push --tags`

#### Update OLM metadata

1. The following instructions amount to mirroring upstream changes in the OLM manifests beneath [openshift/olm](https://github.com/openshift/knative-eventing/tree/master/openshift/olm) in our forks.
2. Create a new `*.clusterserviceversion.yaml` for the upstream release. It’s easiest to copy the previous release’s CSV to a new file and update the name, version, and replaces fields, as well as the version of the operator’s image.
3. Ensure the upstream release’s RBAC policies match what’s in the CSV. Any upstream edits should be carried over.
4. Mirror any upstream manifest changes to CRD’s in the corresponding `*.crd.yaml` file.
5. Update the currentCSV field in the `*.package.yaml` file.
6. Regenerate the `*.catalogsource.yaml` file using the `catalog.sh` script. For example,

```NAME=knative-serving \
DIR=openshift/olm \
~/src/knative-operators/etc/scripts/catalog.sh \
> openshift/olm/knative-serving.catalogsource.yaml
```

#### Tag the repository

#### Get a "LGTM" from QE

#### Get a "LGTM" from Docs

#### Gather release notes from JIRA/GitHub

#### Send a release announcement

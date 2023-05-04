#!/usr/bin/env bash

set -euo pipefail

repo_root_dir=$(dirname "$(realpath "${BASH_SOURCE[0]}")")/..

"${repo_root_dir}/hack/update-deps.sh" || exit 1
git apply "${repo_root_dir}/openshift/patches/005-k8s-min.patch"
git apply "${repo_root_dir}/openshift/patches/018-rekt-test-override-kopublish.patch"
git apply "${repo_root_dir}/openshift/patches/018-rekt-test-image-pod.patch"
git apply "${repo_root_dir}/openshift/patches/020-mutemetrics.patch"

GO111MODULE=off go get -u github.com/openshift-knative/hack/cmd/generate

generate \
  --root-dir "${repo_root_dir}" \
  --generators dockerfile

images_dir="openshift/ci-operator/knative-images"

# This allows having the final images with the expected names
rm -rf "${repo_root_dir}/${images_dir}/mtbroker_filter"
rm -rf "${repo_root_dir}/${images_dir}/mtbroker_ingress"
mv "${repo_root_dir}/${images_dir}/filter" "${repo_root_dir}/${images_dir}/mtbroker_filter"
mv "${repo_root_dir}/${images_dir}/ingress" "${repo_root_dir}/${images_dir}/mtbroker_ingress"

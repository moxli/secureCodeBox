#!/usr/bin/env bash

# SPDX-FileCopyrightText: 2020 iteratec GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Official uninstall script for the secureCodeBox
#
# Removes all available resources (scanners, demo-apps, hooks, operator) and namespaces
#
# For more information see https://docs.securecodebox.io/

set -eu
shopt -s extglob

# @see: http://wiki.bash-hackers.org/syntax/shellvars
[ -z "${SCRIPT_DIRECTORY:-}" ] \
  && SCRIPT_DIRECTORY="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"

BASE_DIR=$(dirname "${SCRIPT_DIRECTORY}")

SCB_SYSTEM_NAMESPACE='securecodebox-system'
SCB_DEMO_NAMESPACE='demo-apps'
SCB_NAMESPACE='default'

function uninstallResources() {
  local resource_directory="$1"
  local namespace="$2"

  local resources=()
  for path in "$resource_directory"/*; do
    [ -d "${path}" ] || continue # skip if not a directory
    local directory
    directory="$(basename "${path}")"
    resources+=("${directory}")
  done

  for resource in "${resources[@]}"; do
    local resource_name="${resource//+([_])/-}" # Necessary because ssh_scan is called ssh-scan
    helm uninstall "$resource_name" -n "$namespace" || true
  done
}

helm -n "$SCB_SYSTEM_NAMESPACE" uninstall securecodebox-operator || true

uninstallResources "$BASE_DIR/demo-apps" "$SCB_DEMO_NAMESPACE"
uninstallResources "$BASE_DIR/scanners" "$SCB_NAMESPACE"
uninstallResources "$BASE_DIR/hooks" "$SCB_NAMESPACE"

kubectl delete namespaces "$SCB_SYSTEM_NAMESPACE" || true

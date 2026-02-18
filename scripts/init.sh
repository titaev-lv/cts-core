#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONF_DIR="${SCRIPT_DIR}/../conf"
EXAMPLE_CFG="${CONF_DIR}/config.example.yaml"
TARGET_CFG="${CONF_DIR}/config.yaml"

if [[ ! -f "${TARGET_CFG}" ]]; then
	if [[ ! -f "${EXAMPLE_CFG}" ]]; then
		echo "config.example.yaml not found at ${EXAMPLE_CFG}" >&2
		exit 1
	fi

	cp "${EXAMPLE_CFG}" "${TARGET_CFG}"
	echo "Created ${TARGET_CFG} from ${EXAMPLE_CFG}"
else
	echo "${TARGET_CFG} already exists, skipping"
fi

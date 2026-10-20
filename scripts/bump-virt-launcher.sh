#!/usr/bin/env bash
set -euo pipefail

# Fetch the latest KubeVirt release tag from GitHub.
fetch_latest_release() {
  local latest
  latest="$(gh release view --repo kubevirt/kubevirt --json 'tagName' -q '.tagName')"
  if [[ -z "$latest" ]]; then
    echo "ERROR: Could not fetch latest KubeVirt release." >&2
    exit 1
  fi
  # Strip leading 'v' if present
  echo "${latest#v}"
}

# Extract the current version from constants.go.
get_current_version() {
  local constants_file="pkg/util/constants.go"
  if [[ ! -f "$constants_file" ]]; then
    echo "ERROR: $constants_file not found." >&2
    exit 1
  fi
  local version
  version="$(grep -oP 'virt-launcher:\K[^"]+' "$constants_file" | head -1)"
  if [[ -z "$version" ]]; then
    echo "ERROR: Could not extract current version from $constants_file." >&2
    exit 1
  fi
  echo "${version#v}"
}

# Check whether the latest release image exists on quay.io.
image_exists() {
  local version="$1"
  local escaped="${version//./\\.}"
  local body
  body="$(curl -fsSL \
    "https://quay.io/api/v1/repository/kubevirt/virt-launcher/tag/?specificTag=v${version}&onlyActiveTags=true")" || return 1
  # Empty result returns {"tags": [], ...}; a published tag has a name entry.
  grep -Eq "\"name\":[[:space:]]*\"v${escaped}\"" <<<"$body"
}

main() {
  local current_version
  current_version="$(get_current_version)"

  local latest_version
  latest_version="$(fetch_latest_release)"

  echo "Current version: v${current_version}"
  echo "Latest release:  v${latest_version}"

  local up_to_date=true
  for f in pkg/util/constants.go README.md docs/configuration.md pkg/podman/container.go; do
    if ! grep -q "quay.io/kubevirt/virt-launcher:v${latest_version}" "$f"; then
      up_to_date=false
    fi
  done
  if [[ "$current_version" == "$latest_version" && "$up_to_date" == "true" ]]; then
    echo "Already up to date. Exiting."
    exit 0
  fi

  # Verify the latest release actually has a published virt-launcher image.
  if ! image_exists "$latest_version"; then
    echo "WARNING: No published virt-launcher image found for v${latest_version}. Skipping bump." >&2
    exit 0
  fi

  # Skip if a PR for this version already exists.
  if gh pr list --base main --state open --json 'headRefName' -q '.[].headRefName' | grep -qx "virt-launcher-v${latest_version}"; then
    echo "PR already exists for v${latest_version}. Skipping."
    exit 0
  fi

  echo "Creating branch: virt-launcher-v${latest_version}"
  git checkout -b "virt-launcher-v${latest_version}"

  # Rewrite the image reference wherever it appears, regardless of which
  # version each file currently mentions, so drifted files are resynced.
  sed -i -E "s|quay\.io/kubevirt/virt-launcher:v[0-9]+\.[0-9]+\.[0-9]+|quay.io/kubevirt/virt-launcher:v${latest_version}|g" \
    pkg/util/constants.go docs/configuration.md pkg/podman/container.go
  # README: bump only the canonical default (standalone line), leave the
  # --launcher-image override example alone.
  sed -i -E "s|^quay\.io/kubevirt/virt-launcher:v[0-9]+\.[0-9]+\.[0-9]+|quay.io/kubevirt/virt-launcher:v${latest_version}|" README.md

  local subject="Update the default virt-launcher image to latest release v${latest_version}"
  git add pkg/util/constants.go README.md docs/configuration.md pkg/podman/container.go
  git commit -m "${subject}" -s

  # Push the branch and create the PR.
  git push origin "virt-launcher-v${latest_version}"

  gh pr create \
    --base main \
    --head "virt-launcher-v${latest_version}" \
    --title "$subject" \
    --body "Bump default virt-launcher image from **v${current_version}** to **v${latest_version}**.

**Automated bump triggered by weekly CI workflow.**

Files changed:
- \`pkg/util/constants.go\` — canonical constant
- \`README.md\` — documentation
- \`docs/configuration.md\` — CLI flag defaults table
- \`pkg/podman/container.go\` — qemu.conf comment"

  echo "Pull request created successfully."
}

main "$@"

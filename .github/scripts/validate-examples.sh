#!/usr/bin/env bash
# Runs `terraform init -backend=false` and `terraform validate` in every directory under examples/
# that holds a .tf file, against the provider built from this checkout.
#
# The registry docs embed these files without parsing them, so this is what catches an example
# a schema change broke. The local build is served from a filesystem mirror, so modules and
# other providers still come from the registry. Needs go, terraform and network access.
set -euo pipefail

cd "$(dirname "$0")/../.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Any version the examples' constraints accept; the mirror holds only this build.
version="$(cat VERSION).0"
platform="$(go env GOOS)_$(go env GOARCH)"
mirror="$work/mirror/registry.terraform.io/monte-carlo-data/montecarlo/$version/$platform"
mkdir -p "$mirror" "$work/plugin-cache"
go build -o "$mirror/terraform-provider-montecarlo_v$version" .

cat >"$work/terraformrc" <<EOF
provider_installation {
  filesystem_mirror {
    path    = "$work/mirror"
    include = ["registry.terraform.io/monte-carlo-data/montecarlo"]
  }
  direct {
    exclude = ["registry.terraform.io/monte-carlo-data/montecarlo"]
  }
}
plugin_cache_dir = "$work/plugin-cache"
EOF
export TF_CLI_CONFIG_FILE="$work/terraformrc"

failed=0
i=0
while IFS= read -r dir; do
  i=$((i + 1))
  # A data dir per example keeps .terraform/ out of the checkout.
  export TF_DATA_DIR="$work/data/$i"
  if ! terraform -chdir="$dir" init -backend=false -input=false -no-color >"$work/step.log" 2>&1 ||
    ! terraform -chdir="$dir" validate -no-color >"$work/step.log" 2>&1; then
    echo "FAILED  $dir"
    sed 's/^/        /' "$work/step.log"
    failed=1
  else
    echo "ok      $dir"
  fi
  rm -f "$dir/.terraform.lock.hcl"
done < <(find examples -name '*.tf' -exec dirname {} \; | sort -u)

exit "$failed"

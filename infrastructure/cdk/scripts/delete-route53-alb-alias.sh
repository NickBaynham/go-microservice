#!/usr/bin/env bash
# Delete a Route 53 alias A record created by upsert-route53-alb-alias.sh.
# Idempotent: exits 0 if the record does not exist.
#
# Usage:
#   ./delete-route53-alb-alias.sh <hosted-zone-name> <record-name>
#
# Examples:
#   ./delete-route53-alb-alias.sh example.com dev-api
#   ./delete-route53-alb-alias.sh example.com test-api
#   ./delete-route53-alb-alias.sh example.com api

set -euo pipefail

ZONE_NAME=${1:?usage: zone e.g. example.com}
RECORD_LABEL=${2:?usage: left part of hostname e.g. api (becomes api.zone.name)}

FQDN="${RECORD_LABEL}.${ZONE_NAME}."

HOSTED_ZONE_ID=$(aws route53 list-hosted-zones-by-name \
  --dns-name "${ZONE_NAME}." \
  --query 'HostedZones[0].Id' \
  --output text | cut -d/ -f3)

if [[ -z "$HOSTED_ZONE_ID" || "$HOSTED_ZONE_ID" == "None" ]]; then
  echo "No hosted zone found for ${ZONE_NAME}."
  exit 1
fi

RR_JSON=$(mktemp)
BATCH_JSON=$(mktemp)
trap 'rm -f "$RR_JSON" "$BATCH_JSON"' EXIT

: >"$BATCH_JSON"
aws route53 list-resource-record-sets --hosted-zone-id "$HOSTED_ZONE_ID" --output json >"$RR_JSON"

python3 - "$FQDN" "$RR_JSON" "$BATCH_JSON" <<'PY'
import json, sys

fqdn, rr_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
with open(rr_path) as f:
    data = json.load(f)
for r in data.get("ResourceRecordSets", []):
    if r.get("Name") == fqdn and r.get("Type") == "A" and "AliasTarget" in r:
        batch = {"Changes": [{"Action": "DELETE", "ResourceRecordSet": r}]}
        with open(out_path, "w") as out:
            json.dump(batch, out)
        sys.exit(0)
print(f"No alias A record for {fqdn} — nothing to delete.")
open(out_path, "w").close()
sys.exit(0)
PY

if [[ ! -s "$BATCH_JSON" ]]; then
  exit 0
fi

echo "Deleting Route 53 record: ${FQDN}"
aws route53 change-resource-record-sets --hosted-zone-id "$HOSTED_ZONE_ID" --change-batch "file://$BATCH_JSON"
echo "Done."

#!/usr/bin/env bash
# Upsert a Route 53 alias A record pointing a hostname at the ALB from a CDK/CloudFormation stack.
#
# Usage:
#   ./upsert-route53-alb-alias.sh <hosted-zone-name> <record-name> <cf-stack-name> [region]
#
# Examples (region defaults to us-east-1):
#   ./upsert-route53-alb-alias.sh example.com dev-api GoMicroservice-Dev
#   ./upsert-route53-alb-alias.sh example.com test-api GoMicroservice-Test
#   ./upsert-route53-alb-alias.sh example.com api GoMicroservice-Prod
#
# Prerequisites: AWS CLI configured; Route 53 hosted zone for the domain; stack deployed with ALBDnsName output.

set -euo pipefail

ZONE_NAME=${1:?usage: zone e.g. calgentik.com}
RECORD_LABEL=${2:?usage: left part of hostname e.g. dev-api (becomes dev-api.zone.name)}
STACK_NAME=${3:?usage: CloudFormation stack name e.g. GoMicroservice-Dev}
REGION=${4:-us-east-1}

# Full record name (Route 53 expects FQDN with trailing dot in API)
FQDN="${RECORD_LABEL}.${ZONE_NAME}."

HOSTED_ZONE_ID=$(aws route53 list-hosted-zones-by-name \
  --dns-name "${ZONE_NAME}." \
  --query 'HostedZones[0].Id' \
  --output text | cut -d/ -f3)

if [[ -z "$HOSTED_ZONE_ID" || "$HOSTED_ZONE_ID" == "None" ]]; then
  echo "No hosted zone found for ${ZONE_NAME}. Create the zone in Route 53 first (Console → Route 53 → Hosted zones → Create)."
  exit 1
fi

set +e
ALB_DNS=$(aws cloudformation describe-stacks --stack-name "$STACK_NAME" --region "$REGION" \
  --query "Stacks[0].Outputs[?OutputKey=='ALBDnsName'].OutputValue" \
  --output text 2>/dev/null)
CF_EXIT=$?
set -e

if [[ $CF_EXIT -ne 0 ]]; then
  echo "CloudFormation stack \"$STACK_NAME\" was not found in $REGION (or the request failed)."
  echo "Deploy that environment first so the ALB exists, e.g.:"
  echo "  make aws-up ENV=dev AWS_ACCOUNT_ID=<id> APP_IMAGE=<ecr:tag> ACM_CERT_ARN=<arn> JWT_SECRET=<secret>"
  echo "or:  cd infrastructure/cdk && cdk deploy $STACK_NAME  (with CDK_* env vars set)"
  exit 1
fi

if [[ -z "$ALB_DNS" || "$ALB_DNS" == "None" ]]; then
  echo "Stack $STACK_NAME has no ALBDnsName output (deployment may still be in progress)."
  exit 1
fi

ALB_ZONE=$(aws elbv2 describe-load-balancers --region "$REGION" \
  --query "LoadBalancers[?DNSName=='${ALB_DNS}'].CanonicalHostedZoneId | [0]" \
  --output text)

if [[ -z "$ALB_ZONE" || "$ALB_ZONE" == "None" ]]; then
  echo "Could not resolve CanonicalHostedZoneId for ALB DNS: $ALB_DNS"
  exit 1
fi

# Route 53 alias target DNSName must end with a trailing dot
ALB_DNS_DOT="${ALB_DNS%.}."

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

cat >"$TMP" <<EOF
{
  "Changes": [{
    "Action": "UPSERT",
    "ResourceRecordSet": {
      "Name": "${FQDN}",
      "Type": "A",
      "AliasTarget": {
        "HostedZoneId": "${ALB_ZONE}",
        "DNSName": "${ALB_DNS_DOT}",
        "EvaluateTargetHealth": false
      }
    }
  }]
}
EOF

echo "Hosted zone: $HOSTED_ZONE_ID"
echo "Record:      $FQDN → ALB $ALB_DNS (zone $ALB_ZONE)"
aws route53 change-resource-record-sets --hosted-zone-id "$HOSTED_ZONE_ID" --change-batch "file://$TMP"
echo "Done. DNS propagation can take a few minutes."

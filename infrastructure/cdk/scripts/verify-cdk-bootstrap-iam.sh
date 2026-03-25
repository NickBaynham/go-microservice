#!/usr/bin/env bash
# Fail fast before `cdk deploy` if credentials cannot assume CDK bootstrap roles.
# Asset publishing uses the file-publishing role; without AssumeRole, CDK falls back to the IAM user and S3 denies access.
# Run in CI after configure-aws-credentials, or locally with the same keys as GitHub Actions.
set -euo pipefail

REGION="${AWS_REGION:-us-east-1}"
IDENTITY_JSON=$(aws sts get-caller-identity --output json)
ACCOUNT=$(echo "$IDENTITY_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['Account'])")
ARN=$(echo "$IDENTITY_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['Arn'])")

if [[ -n "${EXPECTED_ACCOUNT_ID:-}" && "$EXPECTED_ACCOUNT_ID" != "$ACCOUNT" ]]; then
  echo "::error::AWS_ACCOUNT_ID secret ($EXPECTED_ACCOUNT_ID) does not match sts get-caller-identity ($ACCOUNT). Fix GitHub secrets."
  exit 1
fi

ROLE_PUBLISH="arn:aws:iam::${ACCOUNT}:role/cdk-hnb659fds-file-publishing-role-${ACCOUNT}-${REGION}"
ROLE_DEPLOY="arn:aws:iam::${ACCOUNT}:role/cdk-hnb659fds-deploy-role-${ACCOUNT}-${REGION}"
ROLE_LOOKUP="arn:aws:iam::${ACCOUNT}:role/cdk-hnb659fds-lookup-role-${ACCOUNT}-${REGION}"

echo "Caller: $ARN"
echo "Region: $REGION"
echo "Checking sts:AssumeRole on CDK bootstrap roles..."

for role_arn in "$ROLE_LOOKUP" "$ROLE_DEPLOY" "$ROLE_PUBLISH"; do
  if ! aws sts assume-role --role-arn "$role_arn" --role-session-name "gha-verify-${RANDOM}" --duration-seconds 900 --output json >/dev/null; then
    echo "::error::Could not assume $role_arn"
    echo "Attach sts:AssumeRole for arn:aws:iam::${ACCOUNT}:role/cdk-hnb659fds-* to this IAM identity and ensure each role's trust policy allows it (see infrastructure/cdk/TROUBLESHOOTING-CDK-IAM.md)."
    exit 1
  fi
  echo "  OK: ${role_arn##*/}"
done

echo "CDK bootstrap IAM checks passed."

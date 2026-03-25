# AWS Setup Guide (CDK)

Everything you need to do once before the pipeline can run.

## Prerequisites

- AWS CLI installed and configured (`aws configure`)
- Node.js 18+ installed (CDK requires it: `brew install node`)
- AWS CDK CLI: `npm install -g aws-cdk`

---

## Step 1 — Request an ACM Certificate

The ALB needs a real TLS certificate for your domain:

```bash
aws acm request-certificate \
  --domain-name api.yourdomain.com \
  --validation-method DNS \
  --region us-east-1
```

Add the DNS validation records ACM provides to your domain registrar. Once validated, copy the certificate ARN — you'll need it in Step 4.

---

## Step 2 — Bootstrap CDK

CDK needs a one-time bootstrap per AWS account/region. This creates the CDKToolkit CloudFormation stack with an S3 bucket and ECR repo for CDK assets — no manual setup required:

```bash
make cdk-bootstrap AWS_ACCOUNT_ID=123456789012
```

---

## Step 3 — Create an IAM user for GitHub Actions

```bash
aws iam create-user --user-name github-actions-go-microservice

for POLICY in \
  AmazonEC2ContainerRegistryFullAccess \
  AmazonECS_FullAccess \
  AmazonSSMFullAccess \
  CloudWatchLogsFullAccess \
  IAMFullAccess \
  AmazonVPCFullAccess \
  ElasticLoadBalancingFullAccess \
  AWSCloudFormationFullAccess; do
  aws iam attach-user-policy \
    --user-name github-actions-go-microservice \
    --policy-arn arn:aws:iam::aws:policy/$POLICY
done

# Save the AccessKeyId and SecretAccessKey from this output:
aws iam create-access-key --user-name github-actions-go-microservice
```

---

## Step 4 — Add GitHub Secrets

Go to **Settings → Secrets and variables → Actions** and add:

| Secret | Value |
|---|---|
| `AWS_ACCESS_KEY_ID` | From Step 3 |
| `AWS_SECRET_ACCESS_KEY` | From Step 3 |
| `AWS_ACCOUNT_ID` | Your 12-digit AWS account ID |
| `ACM_CERTIFICATE_ARN` | From Step 1 |
| `JWT_SECRET` | Run: `openssl rand -hex 32` |

---

## Step 5 — Set up GitHub Environments

Go to **Settings → Environments** and create three environments: `dev`, `test`, `prod`.

Add **Required reviewers** on **`dev`**, **`test`**, and **`prod`** as needed for approval gates before each deploy.

---

## Step 6 — Run the CD pipeline (manual)

Nothing deploys to AWS on merge to `main`. **Pull requests** run unit + integration tests only. To build the image and deploy:

1. **Actions** → **CI/CD Pipeline** → **Run workflow**
2. Select branch **`main`**
3. Choose whether to deploy to **prod** after test (`deploy_to_prod`: **no** or **yes**)

```
unit tests → integration tests → build image → dev → test → (optional) prod
```

---

## Day-to-day demo workflow

```bash
# Deploy via GitHub Actions (manual — branch main):
# Actions → CI/CD Pipeline → Run workflow

# Or deploy locally:
make aws-up ENV=prod \
  APP_IMAGE=123456789012.dkr.ecr.us-east-1.amazonaws.com/go-microservice:latest \
  AWS_ACCOUNT_ID=123456789012 \
  ACM_CERT_ARN=arn:aws:acm:us-east-1:123456789012:certificate/... \
  JWT_SECRET=$(openssl rand -hex 32)

# Run tests against live prod
make aws-test ENV=prod

# Tail logs
make aws-logs ENV=prod

# Tear down when done (zero cost):
# Actions → Teardown stack → choose prod

# Or locally:
make aws-down ENV=prod APP_IMAGE=... AWS_ACCOUNT_ID=... ACM_CERT_ARN=... JWT_SECRET=...
```

---

## Cost estimate

| Resource | Running | Destroyed |
|---|---|---|
| ECS Fargate (1 task, 0.5 vCPU, 1GB) | ~$0.02/hr | $0 |
| ALB | ~$0.02/hr | $0 |
| ECR storage | ~$0.01/mo | ~$0.01/mo |
| CloudWatch logs | ~$0.01/mo | ~$0.01/mo |
| CDKToolkit assets | ~$0.01/mo | ~$0.01/mo |
| **Total while running** | **~$0.05/hr** | — |
| **Total while destroyed** | — | **~$0.03/mo** |

A 2-hour demo costs roughly **$0.10**.
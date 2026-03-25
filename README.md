# go-microservice

A production-ready Go REST API microservice for user management, featuring JWT authentication, MongoDB integration, Docker support, and TLS encryption (self-signed for dev/test, Let's Encrypt for production).

## Project Structure

```
go-microservice/
├── Makefile             # Developer commands
├── cmd/server/          # Entry point
├── internal/
│   ├── auth/            # JWT helpers
│   ├── config/          # Environment config
│   ├── handlers/        # HTTP handlers
│   ├── middleware/      # Auth, roles, rate limiting
│   ├── models/          # Request/response models
│   └── repository/      # MongoDB data layer
├── certs/               # Dev/test self-signed certs (git-ignored)
├── deployments/
│   ├── docker-compose.yml       # Dev stack
│   ├── docker-compose.prod.yml  # Production stack (Nginx + certbot)
│   └── nginx/conf.d/app.conf    # Nginx SSL config
├── internal/tls/        # TLS config loader
├── Dockerfile
├── .env.example
└── go.mod
```

## API Endpoints

### Public
| Method | Path              | Description          |
|--------|-------------------|----------------------|
| GET    | /health           | Health check         |
| POST   | /auth/register    | Register (first user becomes **admin**, rest are **user**; rate-limited) |
| POST   | /auth/login       | Login & get JWT (rate-limited) |

### Protected (JWT required)
| Method | Path         | Description             |
|--------|--------------|-------------------------|
| GET    | /me          | Get current user        |
| PUT    | /users/:id   | Update user (self/admin) |

### Admin only (JWT + role=admin required)
| Method | Path         | Description    |
|--------|--------------|----------------|
| GET    | /users       | List all users |
| GET    | /users/:id   | Get user by ID |
| DELETE | /users/:id   | Delete user    |

## Getting Started

### Prerequisites
- [Homebrew](https://brew.sh) (macOS) — used to install Go
- Docker & Docker Compose (for containerized setup)

### Quickstart (new developers)

```bash
git clone <repo-url>
cd go-microservice
make setup   # installs Go, creates .env, downloads dependencies
make run     # starts HTTPS server at https://localhost:8443
```

That's it! `make setup` handles everything — Go installation, `.env`, dependencies, and self-signed dev certificates.

### Configuration file (`.env`)

Local settings live in a **`.env`** file at the repo root. It is **gitignored** — do not commit secrets.

1. Copy the template and edit values:
   ```bash
   cp .env.example .env
   ```
2. **`.env.example`** in the repo lists every variable with **dummy / safe placeholders** and short comments. Use it as the canonical checklist; the table below describes behavior and defaults.
3. **Production:** use a long random `JWT_SECRET` (e.g. `openssl rand -hex 32`). Match **`DOMAIN`** in `.env` to your real apex domain; set the same **`DOMAIN`** as a **GitHub repository Variable** so CI health checks use `dev-api.<DOMAIN>`, etc.
4. Variables not set in `.env` fall back to built-in defaults where the app defines them (see `internal/config`).

### All available Make commands

| Command | Description |
|---|---|
| `make setup` | Full onboarding — installs Go, creates .env, pulls deps, generates certs |
| `make run` | Start HTTPS server locally on :8443 (self-signed cert) |
| `make build` | Compile binary to `bin/` |
| `make test` | Run all tests |
| `make certs` | Generate self-signed dev/test certificates |
| `make certs-trust` | Trust dev cert in macOS keychain (removes browser warning) |
| `make certs-check` | Show dev cert expiry date |
| `make docker-up` | Start dev stack (HTTPS on :8443) |
| `make docker-down` | Stop dev Docker services |
| `make docker-prod-up` | Start production stack (Nginx + Let's Encrypt) |
| `make docker-prod-down` | Stop production Docker services |
| `make letsencrypt` | Obtain Let's Encrypt cert (requires DOMAIN= and EMAIL=) |
| `make clean` | Remove build artifacts |
| `make help` | List all commands |

### Run with Docker Compose

```bash
make docker-up
```

### ⚠️ GoLand users

GoLand sets `GOROOT` automatically and may point to an old Go installation. If you see `cannot find GOROOT directory`, update it in:

**Settings → Build, Execution, Deployment → Go → GOROOT**

Set it to the output of:
```bash
brew --prefix go
```

## Example Usage

> Note: Use `-k` with curl in dev to skip self-signed cert verification. In production, omit `-k`.

### Register
```bash
curl -k -X POST https://localhost:8443/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice","email":"alice@example.com","password":"secret123"}'
```

### Login
```bash
curl -k -X POST https://localhost:8443/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"secret123"}'
```

### Get current user
```bash
curl -k https://localhost:8443/me \
  -H "Authorization: Bearer <token>"
```

### Health check
```bash
curl -k https://localhost:8443/health
```

## Environment Variables

These are usually set in **`.env`** (loaded at startup). Most map to `internal/config.Config`; **`LISTEN_HTTP`** is read separately for HTTP-only mode behind a proxy. **`DOMAIN`** is for **`make route53-alias`** and GitHub **`DOMAIN`** variable (not read by the Go binary). See **`.env.example`** for a full commented template with dummy values.

| Variable         | Default                   | Description                                      |
|------------------|---------------------------|--------------------------------------------------|
| PORT             | 8080                      | HTTP listen port when TLS is offloaded (Compose prod, ECS) |
| TLS_PORT         | 8443                      | HTTPS port when the app serves TLS (dev / local Docker) |
| LISTEN_HTTP      | _(unset)_                 | If `true` / `1` / `yes`, listen on **PORT** with plain HTTP (used on ECS; TLS at ALB) |
| ENV              | development               | `development`, `test`, `production`, or CDK `dev` / `test` / `prod` |
| MONGO_URI        | mongodb://localhost:27017 | MongoDB connection URI                           |
| MONGO_DB         | userservice               | Database name                                    |
| JWT_SECRET       | change-me-in-production   | JWT signing secret; for **`production` or `prod`**, must be **≥ 32 characters** and not the default |
| JWT_EXPIRE_HOURS | 24                        | Token expiry in hours (**0–8760**; invalid values fall back to 24 outside prod) |
| TLS_CERT         | _(empty)_                 | Cert PEM path; empty + `production`/`prod` ⇒ HTTP on PORT (proxy terminates TLS) |
| TLS_KEY          | _(empty)_                 | Key PEM path; same as TLS_CERT                   |
| DOMAIN           | _(unset)_                 | Public **DNS apex** (e.g. `example.com`). Used by **`make route53-alias`** (`DNS_ZONE` defaults to this). Set the same name as GitHub repo Variable **`DOMAIN`** for CI hostnames. |

Registration ignores any client-supplied role: the **first** account in the database is **`admin`**, every later signup is **`user`**.

## TLS / SSL

This service uses **self-signed certificates** in dev/test and **Let's Encrypt** in production.

### Development & Test (self-signed)

Certs are auto-generated to `./certs/` on first run. No manual steps needed:

```bash
make run        # generates certs automatically, starts HTTPS on :8443
```

Your browser will show a security warning — that's expected. To silence it on macOS:

```bash
make certs-trust   # adds the dev cert to your macOS keychain
```

Test with curl (skip cert verification in dev):
```bash
curl -k https://localhost:8443/health
```

### Production (Let's Encrypt via Nginx)

1. Point your domain's DNS A record to your server IP.

2. Update `deployments/nginx/conf.d/app.conf` — replace `YOUR_DOMAIN_HERE` with your domain.

3. Obtain the certificate (run once on your server):
```bash
make letsencrypt DOMAIN=api.example.com EMAIL=you@example.com
```

4. Start the production stack:
```bash
make docker-prod-up
```

Certbot runs as a sidecar container and **auto-renews** the certificate every 12 hours.

### How it works

| Environment | TLS handled by | App listens | Certificate |
|---|---|---|---|
| `development` | Go directly | HTTPS on **TLS_PORT** (e.g. 8443) | Self-signed (auto-generated) |
| `test` | Go directly | HTTPS on **TLS_PORT** | Self-signed (mounted or generated) |
| `production` (Docker Compose) | Nginx | HTTP on **PORT** (8080) | Let's Encrypt (certbot) |
| `dev` / `test` / `prod` (ECS + ALB) | Application Load Balancer | HTTP on **PORT** (8080); `LISTEN_HTTP=true` | ACM cert on ALB |

In production-style deployments, TLS terminates at **Nginx** or the **ALB**; the Go process serves **plain HTTP** on port 8080 inside the network.

## Swagger / API Documentation

Swagger docs are auto-generated from annotations in the handler code using [swaggo/swag](https://github.com/swaggo/swag).

### Generate & view docs

```bash
make docs   # generates ./docs/ from code annotations
make run    # start the server
```

Then open in your browser:
```
https://localhost:8443/swagger/index.html
```

You can authorize with a JWT directly in the UI — click the **Authorize** button and enter `Bearer <your_token>`.

> Swagger UI is automatically **disabled** when `ENV` is **`production`** or **`prod`** (including CDK).

### Regenerate after changes

Any time you add or modify a handler, regenerate the docs:

```bash
make docs
```

Or it runs automatically as part of `make run` and `make build`.

## Testing

The test suite covers every endpoint — status codes, response keys/values, auth rules, and error cases. The database is cleaned before and after each run so tests are fully isolated and repeatable.

**Integration tests** live behind the Go build tag **`integration`**. `make test`, `make test-integration`, and `make test-integration-local` pass that tag for you. Plain `go test ./...` runs **unit tests only** (no live server required).

### Run all tests (pipeline-safe)

```bash
make test
```

This runs unit tests, then spins up a dedicated Docker environment (separate DB on port 9443), runs the integration tests (`-tags=integration`), then tears everything down. Safe to run in CI pipelines.

### Run integration tests against your local server

Start the API with the **same** `MONGO_DB` and `JWT_SECRET` as the test command. Defaults match plain `make run` (`MONGO_DB=userservice`):

```bash
JWT_SECRET=change-me-in-production make run
make test-integration-local
```

`make test-integration-local` clears the **entire** `users` collection in that database before tests so the first registration becomes admin. To avoid touching your main dev database, use a separate DB for **both** the server and tests, e.g. `MONGO_DB=userservice_test make run` and `MONGO_DB=userservice_test make test-integration-local`.

### Run unit tests only

```bash
make test-unit
```

### Debug a test failure

Start the isolated test environment and leave it running so you can inspect it:

```bash
make docker-test-up               # starts on port 9443
make test-integration-local       # run tests against it
make docker-test-down             # clean up when done
```

### Test environment variables

| Variable | Default | Description |
|---|---|---|
| `TEST_HOST` | `localhost` | Server host |
| `TEST_PORT` | `8443` | Server port |
| `TEST_SCHEME` | `https` | `http` or `https` |
| `TEST_SKIP_TLS_VERIFY` | `true` | Skip cert check (set `false` for prod certs) |
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB to clean before/after tests |
| `MONGO_DB` | `userservice` (override on server and tests together if needed) | Database name — must match the running server; tests delete all users in this DB before the suite |
| `JWT_SECRET` | _(see Makefile)_ | Must match the server for expired-JWT tests |
| `TEST_HTTP_BASE_URL` | _(unset)_ | Optional `http://localhost:8080` for an extra plain-HTTP `/health` check |

### Pipeline usage (GitHub Actions example)

```yaml
- name: Run tests
  run: make test
```

The `make test` target is fully self-contained — it builds the Docker image, runs all tests, and cleans up automatically.

---

## Deploying to AWS

This service deploys to AWS ECS Fargate via AWS CDK. The infrastructure is defined in Go in `infrastructure/cdk/` and managed entirely through `make` commands.

### Architecture

```
Internet → ALB (HTTPS/443) → ECS Fargate Task (app :8080 HTTP, TLS only on ALB)
                                 ├── app container  (go-microservice)
                                 └── mongo container (MongoDB sidecar)
```

The shared **ECR** repository `go-microservice` is **looked up** by CDK (not created per stack). CI creates the repository before the first image push if it does not exist.

All infrastructure is created and destroyed on demand — you pay only when the service is running (~$0.05/hr). See the cost breakdown at the end of this section.

### Prerequisites

- AWS CLI configured (`aws configure`)
- Node.js 18+ (`brew install node`)
- AWS CDK CLI (`npm install -g aws-cdk`)
- A domain in Route 53

### ⚠️ New AWS account — clean bootstrap required

If you have previously used CDK in this AWS account, you may have a stale `CDKToolkit` CloudFormation stack. Before bootstrapping, check for and clean up any leftover resources:

```bash
# Check if CDKToolkit stack already exists
aws cloudformation describe-stacks \
  --stack-name CDKToolkit \
  --region us-east-1 2>/dev/null | grep StackStatus
```

If it exists and is in a broken state (`UPDATE_ROLLBACK_COMPLETE`, `ROLLBACK_COMPLETE`):

```bash
# 1. Delete any leftover CDK S3 bucket (empty it first in the console if it has contents)
aws s3 rb s3://cdk-hnb659fds-assets-<your-account-id>-us-east-1 --force 2>/dev/null

# 2. Delete the broken stack
aws cloudformation delete-stack --stack-name CDKToolkit --region us-east-1
aws cloudformation wait stack-delete-complete --stack-name CDKToolkit --region us-east-1
```

Then proceed with the steps below.

### Step 1 — Request an ACM certificate

The ALB requires a trusted TLS certificate. Request one for your API subdomain:

```bash
aws acm request-certificate \
  --domain-name api.yourdomain.com \
  --validation-method DNS \
  --region us-east-1
```

Get the DNS validation record ACM needs:

```bash
aws acm describe-certificate \
  --certificate-arn <your-cert-arn> \
  --region us-east-1 \
  --query 'Certificate.DomainValidationOptions'
```

Add the returned CNAME to your Route 53 hosted zone:

| Field | Value |
|---|---|
| Record name | The `Name` from ACM output (e.g. `_abc123.api.yourdomain.com`) |
| Record type | CNAME |
| Value | The `Value` from ACM output (e.g. `_xyz456.acm-validations.aws`) |

Verify DNS has propagated:

```bash
dig _<validation-token>.api.yourdomain.com CNAME
```

Monitor until the certificate is issued:

```bash
watch -n 30 'aws acm describe-certificate \
  --certificate-arn <your-cert-arn> \
  --region us-east-1 \
  --query Certificate.Status'
```

### Step 2 — Bootstrap CDK

Run once per AWS account/region. This creates the `CDKToolkit` CloudFormation stack that CDK uses to manage deployments:

```bash
cd infrastructure/cdk

export CDK_ENV=dev
export CDK_ACCOUNT=<your-aws-account-id>
export CDK_APP_IMAGE=placeholder
export CDK_CERT_ARN=<your-cert-arn>
export CDK_JWT_SECRET=placeholder
export JSII_SILENCE_WARNING_UNTESTED_NODE_VERSION=1

cdk bootstrap aws://<your-aws-account-id>/us-east-1
```

You should see `✅ Environment aws://<account>/us-east-1 bootstrapped`.

### Step 3 — Create a GitHub Actions IAM user

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

# Save the output — you'll need these for GitHub Secrets
aws iam create-access-key --user-name github-actions-go-microservice
```

#### CDK asset publishing (required for `cdk deploy` in CI)

Modern **CDK v2 bootstrap** stores assets in an S3 bucket (`cdk-hnb659fds-assets-<account>-<region>`) and uses IAM roles whose names start with `cdk-hnb659fds-`. The managed policies in Step 3 do **not** grant `sts:AssumeRole` on those roles or S3 access to that bucket, so **`cdk deploy` from GitHub Actions can fail** during the asset publishing phase with *access denied* on the staging bucket or *cannot assume* the file-publishing role.

Attach an **inline policy** to the same IAM user (replace `REPLACE_ACCOUNT_ID`, `REPLACE_REGION`, and the username if needed):

```bash
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
REGION=us-east-1   # same region as CDK / workflow env AWS_REGION

sed -e "s/REPLACE_ACCOUNT_ID/${ACCOUNT_ID}/g" \
    -e "s/REPLACE_REGION/${REGION}/g" \
    infrastructure/cdk/iam/github-actions-cdk-bootstrap-policy.json > /tmp/gha-cdk-bootstrap-policy.json

aws iam put-user-policy \
  --user-name github-actions-go-microservice \
  --policy-name GitHubActionsCDKBootstrap \
  --policy-document file:///tmp/gha-cdk-bootstrap-policy.json
```

Confirm **CDK is bootstrapped** in that account/region (`cdk bootstrap aws://${ACCOUNT_ID}/${REGION}`). If the bucket or role names in your account differ (custom bootstrap), inspect the **CDKToolkit** CloudFormation stack outputs and extend the policy to match.

**If `cdk deploy` still fails** with *could not be used to assume* `cdk-hnb659fds-*-role` or *Bucket exists, but we dont have access*:

1. **Confirm the policy is attached** to the same IAM user as your GitHub secrets:  
   `aws iam list-user-policies --user-name github-actions-go-microservice`
2. **Test role assumption** with those credentials (same access keys as in GitHub):  
   `aws sts assume-role --role-arn arn:aws:iam::ACCOUNT_ID:role/cdk-hnb659fds-file-publishing-role-ACCOUNT_ID-us-east-1 --role-session-name cdk-test`  
   If this fails, fix the role’s **trust policy** so your IAM user (or `arn:aws:iam::ACCOUNT_ID:root`) can assume it, and ensure no **permission boundary** or **Organizations SCP** blocks `sts:AssumeRole`.
3. **Asset uploads** use the file-publishing role; direct S3 calls without a successful assume often hit “no access” because the bootstrap bucket policy allows that role—not your IAM user. Fixing **AssumeRole** + the inline policy above usually resolves it.
4. If the staging bucket uses **SSE-KMS**, add `kms:Decrypt` / `kms:GenerateDataKey` for that key (see KMS key on the bucket in S3 console).

More detail: **`infrastructure/cdk/TROUBLESHOOTING-CDK-IAM.md`**. The workflow runs **`infrastructure/cdk/scripts/verify-cdk-bootstrap-iam.sh`** before `cdk deploy` so AssumeRole failures surface immediately.

### Step 4 — Add GitHub Secrets

In your repo go to **Settings → Secrets and variables → Actions** and add:

| Secret | Where to get it |
|---|---|
| `AWS_ACCESS_KEY_ID` | Output of Step 3 |
| `AWS_SECRET_ACCESS_KEY` | Output of Step 3 |
| `AWS_ACCOUNT_ID` | Your 12-digit AWS account ID |
| `ACM_CERTIFICATE_ARN` | Output of Step 1 |
| `JWT_SECRET` | Run: `openssl rand -hex 32` |

### Step 5 — Set up GitHub Environments

In your repo go to **Settings → Environments** and create three environments: `dev`, `test`, `prod`.

Add **Required reviewers** on **`dev`**, **`test`**, and **`prod`** if you want an approval gate before each deploy (recommended). Prod teardown via **Teardown stack** also respects the **`prod`** environment rules.

### Step 6 — Update environment config files

Edit `infrastructure/cdk/environments/dev.env`, `test.env`, and `prod.env` and fill in your values:

```bash
# Uncomment and fill in:
# export CDK_ACCOUNT=<your-aws-account-id>
# export CDK_CERT_ARN=arn:aws:acm:us-east-1:<account>:certificate/...
```

### Step 7 — First deploy (manual workflow)

CDK deploy **does not** run on merge to `main` (no automatic ECR build or deploy). Run it when you want:

1. **Actions** → **CI/CD Pipeline** → **Run workflow**
2. Branch: **`main`**
3. **Deploy to prod after test:** **`no`** for dev/test only, or **`yes`** to continue to prod after the test deploy (prod still needs **prod** environment approval if configured)

```
unit tests → integration tests → build & push ECR → [approve "dev"] → dev → [approve "test"] → test → (optional) [approve "prod"] → prod
```

Configure **Settings → Environments** with **required reviewers** on **`dev`**, **`test`**, and **`prod`** as you prefer. **Dev and test stacks are left running** after smoke tests by default (no extra variables). To **auto-destroy** after a job, set repository **Variables** **`DESTROY_DEV_AFTER_DEPLOY`** or **`DESTROY_TEST_AFTER_DEPLOY`** to **`true`**. Tear down manually via **Actions → Teardown stack** when you are done.

**Pull requests** to `main` still run **unit** and **integration** tests only (no AWS build/deploy).

Or deploy manually:

```bash
cd infrastructure/cdk
source environments/prod.env

export CDK_APP_IMAGE=<ecr-url>:<tag>
export CDK_JWT_SECRET=$(openssl rand -hex 32)
export JSII_SILENCE_WARNING_UNTESTED_NODE_VERSION=1

cdk deploy GoMicroservice-Prod --require-approval never
```

### Step 8 — Add the API subdomain to Route 53

After deploying, get the ALB DNS name from the CDK outputs:

```bash
aws cloudformation describe-stacks \
  --stack-name GoMicroservice-Prod \
  --region us-east-1 \
  --query "Stacks[0].Outputs[?OutputKey=='ALBDnsName'].OutputValue" \
  --output text
```

Then add an Alias A record in Route 53:

| Field | Value |
|---|---|
| Record name | `api.yourdomain.com` |
| Record type | A |
| Route traffic to | Alias to ALB |
| ALB DNS name | Output from above |

**AWS Console (one record at a time)**  
1. **Route 53 → Hosted zones →** your zone (**`calgentik.com`**).  
2. **Create record**.  
3. **Record name:** `dev-api` (Route 53 appends the zone → `dev-api.calgentik.com`). For prod use `api` if you want `api.calgentik.com`.  
4. **Record type:** **A**.  
5. Turn **Alias** **on**.  
6. **Route traffic to:** **Application and Classic Load Balancer** → region **us-east-1** → pick the load balancer that belongs to **that** stack (name often contains `go-microservice-dev`, `test`, or `prod`).  
7. **Create records**.  
Repeat for **`test-api`** using the **test** stack’s ALB, and keep **`api`** pointing at the **prod** ALB.

**Makefile (wraps the same script):**

```bash
# DOMAIN=example.com in .env (or DNS_ZONE=... to override once)
make route53-alias ENV=dev
make route53-alias ENV=test
make route53-alias ENV=prod
```

**`DNS_ZONE`** defaults to **`DOMAIN`** from `.env` (apex zone, e.g. `example.com`).

**Prerequisite:** the **CDK stack must already exist** in AWS (e.g. `make aws-up ENV=dev …` or a successful **`cdk deploy GoMicroservice-Dev`**). If you see *Stack … does not exist*, deploy that environment first; the script only reads the ALB DNS from CloudFormation outputs.

**Or call the script directly** — after each stack exists and has `ALBDnsName` output:

```bash
cd infrastructure/cdk/scripts
./upsert-route53-alb-alias.sh calgentik.com dev-api   GoMicroservice-Dev
./upsert-route53-alb-alias.sh calgentik.com test-api  GoMicroservice-Test
./upsert-route53-alb-alias.sh calgentik.com api       GoMicroservice-Prod
```

(`GoMicroservice-Dev` / `Test` / `Prod` are the default CDK stack IDs from `CDK_ENV`.)

**Domain name (single source of truth):** set **`DOMAIN`** to your zone apex in **`.env`** (see **`.env.example`**) for local **`make route53-alias`**. For CI, set the same value as a **repository Variable** **`DOMAIN`** under **Settings → Secrets and variables → Actions → Variables** (the workflow defaults to `calgentik.com` if unset).

**Remove an alias** (e.g. before or after tearing down a stack): `make route53-alias-delete ENV=prod` (uses labels **`dev-api`**, **`test-api`**, **`api`** for dev / test / prod). The **Teardown stack** workflow runs this automatically **before** `cdk destroy`. The **CI/CD Pipeline** **`deploy-prod`** job runs **`make route53-alias ENV=prod`** after a successful prod deploy so **`api.${DOMAIN}`** points at the prod ALB. Your IAM user/role for GitHub Actions needs Route 53 permissions to list zones and change records.

The **dev** and **test** jobs in GitHub Actions run **`TestAPISmoke`** (HTTP `/health` only) against the stack’s **ALB DNS name** from CDK outputs (with TLS verification skipped for that hostname). The full **`TestAPI`** suite needs a **`MONGO_URI` reachable from the test process** (Docker integration job uses a local Mongo); ECS Mongo runs only inside the task, so the GitHub Actions runner cannot run the full suite against a deployed stack without a separate DB endpoint (e.g. Atlas) or a runner in your VPC. **Route 53** aliases such as **`dev-api.${DOMAIN}`** / **`test-api.${DOMAIN}`** are optional in CI until you want hostname-based checks; create them with **`make route53-alias`** after the stack exists. **Prod** is typically **`api.${DOMAIN}`**. **Dev** / **test** / **prod** are separate stacks (separate ALBs). Public hostnames need a **Route 53** alias to that stack’s ALB and **ACM** coverage for strict TLS.

### Day-to-day demo workflow

```bash
# Deploy via GitHub Actions (manual — branch main, set "Deploy to prod" if needed)
# Actions → CI/CD Pipeline → Run workflow

# Run integration tests against live prod
make aws-test ENV=prod

# Tail CloudWatch logs
make aws-logs ENV=prod

# Tear down when done (zero cost) — pick dev, test, or prod
# Actions → Teardown stack → Run workflow → environment
```

### Cost estimate

| Resource | While running | While destroyed |
|---|---|---|
| ECS Fargate (0.5 vCPU, 1GB) | ~$0.02/hr | $0 |
| ALB | ~$0.02/hr | $0 |
| ECR image storage | ~$0.01/mo | ~$0.01/mo |
| CloudWatch logs | ~$0.01/mo | ~$0.01/mo |
| CDKToolkit assets | ~$0.01/mo | ~$0.01/mo |
| **Total running** | **~$0.05/hr** | — |
| **Total destroyed** | — | **~$0.03/mo** |

A 2-hour demo costs roughly **$0.10**.
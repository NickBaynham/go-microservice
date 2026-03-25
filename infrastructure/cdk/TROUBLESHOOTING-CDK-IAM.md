# CDK deploy from GitHub Actions: IAM errors

Symptoms:

- `current credentials could not be used to assume '.../cdk-hnb659fds-*-role-...'`
- `Bucket named 'cdk-hnb659fds-assets-...' exists, but we dont have access to it`

The second error is usually a **side effect** of the first: CDK must **assume** the bootstrap **file-publishing role** to upload assets to the staging bucket. If AssumeRole fails, CDK falls back to your IAM user, and the bucket policy typically **denies** that path.

## 1. Identity policy on the GitHub IAM user (or role)

Attach the inline policy from `iam/github-actions-cdk-bootstrap-policy.json` (after `sed` replace `REPLACE_ACCOUNT_ID` / `REPLACE_REGION`), or ensure an equivalent policy allows:

- `sts:AssumeRole` on `arn:aws:iam::<account>:role/cdk-hnb659fds-*`
- S3 actions on `cdk-hnb659fds-assets-<account>-<region>` (needed if your workflow ever uploads without assuming the role; CDK normally uses the role after AssumeRole succeeds)

## 2. Trust policy on each bootstrap role

The roles `cdk-hnb659fds-lookup-role-*`, `cdk-hnb659fds-deploy-role-*`, and `cdk-hnb659fds-file-publishing-role-*` must **trust** the principal that runs `cdk deploy`.

Default CDK bootstrap trust often includes `arn:aws:iam::<account>:root`, which allows any IAM principal in the account to assume the role **if** they have `sts:AssumeRole` in their IAM policy.

If your roles were customized or you use a **narrow** trust, add your GitHub Actions IAM user ARN explicitly:

```json
{
  "Effect": "Allow",
  "Principal": {
    "AWS": "arn:aws:iam::<account-id>:user/github-actions-go-microservice"
  },
  "Action": "sts:AssumeRole"
}
```

(Replace user name and account ID.)

## 3. Verify locally with the same credentials as GitHub

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=us-east-1

export EXPECTED_ACCOUNT_ID=<12-digit-account>
bash infrastructure/cdk/scripts/verify-cdk-bootstrap-iam.sh
```

If this fails, fix IAM before re-running Actions.

## 4. Other blockers

- **Permission boundary** on the IAM user: must allow `sts:AssumeRole` to those role ARNs.
- **AWS Organizations SCP**: must not deny `sts:AssumeRole` or S3 on the bucket.
- **Wrong `AWS_ACCOUNT_ID` secret**: must match `aws sts get-caller-identity` for the access keys in GitHub.

## 5. Re-bootstrap (last resort)

Only if the bootstrap stack is broken or from a very old CDK version:

```bash
cdk bootstrap aws://<account>/us-east-1
```

Review the [CDK bootstrap documentation](https://docs.aws.amazon.com/cdk/v2/guide/bootstrapping.html) before using `--force`.

# OCI CI Deploy User — Setup Runbook

One-time setup that lets GitHub Actions (`deploy-staging.yaml` /
`deploy-production.yaml`) authenticate to OCI and roll out the SimaOps Helm
charts to the OKE cluster, using a **dedicated least-privilege user** rather
than a human/admin identity.

> The deploy workflows use `oracle-actions/configure-kubectl-oke@v1.5.0`,
> which reads the standard `OCI_CLI_*` environment variables. This runbook
> produces the values for the GitHub secrets listed in
> [`deployment.md`](./deployment.md#required-github-secrets).

## Concrete values for this tenancy

| Item | Value |
|---|---|
| Tenancy OCID | `ocid1.tenancy.oc1..aaaaaaaah7nhb32iaac6le5fwjb732bjie6zbpyyt5sihco7videbezloina` |
| Region | `ap-singapore-1` |
| Staging cluster (`simaops-staging-oke`) | `ocid1.cluster.oc1.ap-singapore-1.aaaaaaaaaoyfywqc7bidcp6pqpj4dr3zig76t6vynw26tvkykcfhbwjcz5ia` |
| Cluster compartment | `ocid1.compartment.oc1..aaaaaaaawkcxmr6c2ivptiis4pko4d2cfc6cfdrti4qui324xu7s3sp6j6ra` |

`COMP=ocid1.compartment.oc1..aaaaaaaawkcxmr6c2ivptiis4pko4d2cfc6cfdrti4qui324xu7s3sp6j6ra`

## 1. Create the CI deploy user + group

```bash
oci iam user create --name simaops-ci-deploy \
  --description "GitHub Actions deploy (Helm → OKE)"
# → note the returned user OCID as CI_USER

oci iam group create --name simaops-ci-deploy-grp \
  --description "CI deploy group"
# → note the returned group OCID as CI_GROUP

oci iam group add-user --user-id "$CI_USER" --group-id "$CI_GROUP"
```

## 2. Attach a least-privilege policy

The workflow only needs to read the cluster and generate a kubeconfig token;
the in-cluster Helm operations are then governed by Kubernetes RBAC (the
generated token maps to an OKE RBAC identity). Per the OKE policy reference,
`CreateKubeconfig` requires the **`CLUSTER_USE`** permission — i.e. the `use`
verb on `clusters`. The minimal policy is therefore a single statement:

```bash
oci iam policy create \
  --compartment-id "$COMP" \
  --name simaops-ci-deploy-policy \
  --description "Least-privilege CI deploy: generate OKE kubeconfig token" \
  --statements '[
    "Allow group simaops-ci-deploy-grp to use clusters in compartment id '"$COMP"'"
  ]'
```

> `use clusters` covers `CreateKubeconfig` (CLUSTER_USE), which is all
> `configure-kubectl-oke` calls. You can substitute the aggregate
> `use cluster-family` if you also want the user to inspect node pools/work
> requests, but it is not required for deployment. In-cluster authorization
> (creating/upgrading Deployments, Services, etc. via Helm) is governed by
> **Kubernetes RBAC**, not OCI IAM — the kubeconfig token maps to a subject
> via the cluster's native mapping. If Helm gets `forbidden` errors, bind that
> subject to a cluster role (see step 5).

## 3. Generate an API signing key for the CI user

```bash
openssl genrsa -out simaops-ci-deploy.pem 2048
chmod 600 simaops-ci-deploy.pem
openssl rsa -pubout -in simaops-ci-deploy.pem -out simaops-ci-deploy-public.pem

oci iam user api-key upload --user-id "$CI_USER" \
  --key-file simaops-ci-deploy-public.pem
# → note the returned fingerprint as CI_FINGERPRINT
```

## 4. Populate GitHub secrets

Run from the repo root (uses the `gh` CLI, authenticated to the repo).
`OCI_CLI_KEY_CONTENT` must be the **full PEM contents**, including the
`-----BEGIN/END-----` lines.

```bash
TENANCY=ocid1.tenancy.oc1..aaaaaaaah7nhb32iaac6le5fwjb732bjie6zbpyyt5sihco7videbezloina
REGION=ap-singapore-1
CLUSTER=ocid1.cluster.oc1.ap-singapore-1.aaaaaaaaaoyfywqc7bidcp6pqpj4dr3zig76t6vynw26tvkykcfhbwjcz5ia

gh secret set OCI_CLI_USER        --body "$CI_USER"
gh secret set OCI_CLI_TENANCY     --body "$TENANCY"
gh secret set OCI_CLI_FINGERPRINT --body "$CI_FINGERPRINT"
gh secret set OCI_CLI_REGION      --body "$REGION"
gh secret set OCI_CLI_KEY_CONTENT --body "$(cat simaops-ci-deploy.pem)"
gh secret set OCI_CLUSTER_OCID    --body "$CLUSTER"

# Production (when a prod cluster exists) — separate key + cluster:
# gh secret set OCI_CLI_KEY_CONTENT_PROD --body "$(cat simaops-ci-deploy-prod.pem)"
# gh secret set OCI_CLUSTER_OCID_PROD    --body "<prod-cluster-ocid>"
```

Then delete the local private key (`rm simaops-ci-deploy.pem*`) — it now lives
only in the GitHub secret store.

## 5. (Only if Helm hits `forbidden`) bind the CI subject in-cluster

OKE maps the generated token to a Kubernetes user. If the deploy step fails
with RBAC `forbidden`, grant the subject the rights Helm needs in the
`simaops` namespace (or cluster-wide if charts create namespaces):

```bash
kubectl create clusterrolebinding simaops-ci-deploy \
  --clusterrole=cluster-admin \
  --user="<subject-from-the-forbidden-error>"
```

Prefer a scoped `Role`/`RoleBinding` in `simaops` over `cluster-admin` once
the exact required verbs are known.

## 6. Verify

Trigger a deploy by pushing any commit to `main` (or re-run the latest
**Deploy Staging** run). The `Configure kubectl for OKE` step should now
succeed, followed by four `helm upgrade --install` operations. Confirm:

```bash
kubectl get pods -n simaops          # all Running on the new image tag
helm list -n simaops                 # 4 releases, recent REVISION bump
```

## Security notes

- The CI user has **no console password** and only an API key — it cannot log
  into the OCI console.
- Scope is limited to `use cluster-family` in the cluster's compartment; it
  cannot touch other compartments, billing, or IAM.
- Rotate the signing key periodically: upload a new key
  (`oci iam user api-key upload`), update `OCI_CLI_KEY_CONTENT`, then delete
  the old key (`oci iam user api-key delete`).
- Keep staging and production keys/clusters separate (the workflows already
  use distinct `*_PROD` secrets).

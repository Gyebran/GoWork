# Milestone 10 — CI

Implementation prepared; workflow verification pending.

## Pipeline contract

`CI` replaces the earlier database-foundation and docker-packaging workflows. It runs on every push, pull request, merge-group event and manual dispatch, including documentation-only changes. Concurrent runs of the same ref cancel obsolete runs. No production secrets, deployment, write token permissions, pull_request_target event or persisted checkout credentials are used.

| Job | Required checks |
|---|---|
| Go checks | Formatting, go vet, race unit tests, build all five binaries |
| OpenAPI contract | OpenAPI 3.0.3 structural/schema validation, resolvable local references, unique YAML keys and operation IDs |
| PostgreSQL integration | Pinned sqlc generation with no modified/deleted/new generated files; full race integration suite against disposable PostgreSQL 16.10 |
| Docker lifecycle | Image build and complete M9 Compose smoke, including persistence, outage probes, normal SIGTERM exit and failed-migration startup gate |
| CI required | Runs even after failures/skips and accepts only success from all four prerequisite jobs |

PostgreSQL waits for Go checks; Docker waits for Go and contract checks. This avoids expensive downstream jobs after an early failure. No job uses continue-on-error. Timeouts bound all jobs. Runner OS is ubuntu-24.04, Go comes from go.mod, and Python contract tooling uses 3.12. Checkout v7.0.1, setup-go v7.0.0 and setup-python v7.0.0 are pinned to full commit SHAs verified against the upstream tag refs. Container tags retain M9's version pinning; this is not a claim of bit-for-bit environment reproducibility.

`CI required` is the stable check name to select in GitHub branch protection/rulesets. The workflow itself enforces the pipeline result; it cannot prevent a direct push or merge unless repository settings require that check. Repository administration settings were not changed or claimed verified through the connector. CI success does not authorize production deployment.

## Run the same checks locally

```sh
make check
make sqlc-check
# Requires a fresh disposable gowork_test database, as documented in M2:
APP_ENV=test TEST_DATABASE_URL='<test-only DSN>' make test-integration
python3 -m venv /tmp/gowork-contract
/tmp/gowork-contract/bin/python -m pip install -r scripts/requirements-contract.txt
PATH=/tmp/gowork-contract/bin:$PATH make contract-check
make docker-smoke
```

Python dependencies, including transitive dependencies, are version-pinned in `scripts/requirements-contract.txt`. To update them deliberately, install the chosen validator/PyYAML versions in a clean Python 3.12 virtual environment, freeze dependencies to that file, then rerun contract checks and CI. Python is CI tooling only and is not added to the application image.

Contract validation deliberately rejects external references before validation and rejects duplicate YAML mapping keys rather than silently overwriting them. The existing document validates with 22 operations. This gate validates the document's structure; complete live API/documentation parity and Swagger delivery remain M11.

## Verification

Local contract validation passed; mutated copies with duplicate keys, incorrect OpenAPI version, unresolved references and remote references all failed. CI success and intentional unit-test failure evidence will be recorded after the actual workflow runs. The failure demonstration uses an isolated branch and does not add a failing test to main.

References: [GitHub immutable action pins](https://docs.github.com/en/actions/reference/security/secure-use), [job dependencies](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-jobs), and [required-check skip handling](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks).

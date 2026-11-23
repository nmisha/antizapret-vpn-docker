# Build and publication

The `CI` workflow builds the 13 images in `versions.json` on native amd64 and
arm64 runners, tests the packaged images, then publishes their combined OCI
indexes. Additional `services-mine` images are not part of this manifest.

## Registries and authorization

- Docker Hub: `nmisha/<repository>`.
- GHCR: `ghcr.io/<github.repository_owner>/<ghcr_repository or repository>`.
- Configure the GitHub Actions secret `DOCKER_TOKEN` with Docker Hub write access.
- Docker Hub login defaults to `nmisha`. The repository variable
  `DOCKERHUB_USERNAME` can override the login account without changing image tags.
- GHCR uses `GITHUB_TOKEN` and job-level `packages: write`. Existing packages must
  grant this repository write access.

No secrets are stored in repository files. This change does not publish images
until the workflow runs with publication enabled.

## Triggers and publication policy

Pushes and pull requests targeting `v6-mine`, `dev`, or `v<number>` run CI when
manifest, service sources, scripts, integration tests, workflow, or root Compose
files change. Publication is enabled only for pushes to `v6-mine`, or a manual
run on `v6-mine` with the `publish` checkbox enabled. PRs and other branches never
publish release images. Build caches may be written for supported branch pushes.

Source changes rebuild the affected image; the shared OpenVPN directory rebuilds
both server and UI. Changes to antizapret, CI scripts, integration tests, workflow
or root Compose rebuild all images. Manifest entries also rebuild when their
metadata changes. Manual runs and new branches build all images.

Tags come from each manifest version and aliases (`latest`, and `6` for the main
image). A source-only release push can replace an existing version tag; bump
`versions.json` when a distinct release version is required. The first workflow
change rebuilds all images, bootstrapping the fork's DNS dependency images.
Later partial builds pin unchanged DNS images from the fork registries by digest.

`tests/integration` was restored from upstream and adapted for the fork's
independent firewall-family updates and Authelia hostname. It is required by the
workflow; do not remove it while keeping the integration job.

## Local Docker Hub publication

```sh
# Preview commands; nothing is built or pushed.
scripts/docker-build.sh --dry-run -s https -v 6.12.3

# Build and publish after logging in with docker login.
scripts/docker-build.sh -s https -v 6.12.3

# Override the local destination namespace explicitly.
DOCKERHUB_NAMESPACE=my-account scripts/docker-build.sh --dry-run -s https -v 6.12.3
```

The local script publishes Docker Hub tags only. CI publishes Docker Hub and
GHCR after integration tests. Review the dry-run output before local publication.

## Local checks

```sh
python -m pip install PyYAML==6.0.3
python -m unittest discover -s scripts/tests -v
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= -pyflakes= .github/workflows/build.yml
```

The tests validate planning, registry destinations, release gates, Bake cache
configuration and dry-run commands. They require jq, Bash, and Docker Buildx,
but do not build or publish images. Full packaged-image tests additionally need
the CI images and Linux Docker; compilation alone does not run those tests.

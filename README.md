# GitHub Webhook DCO Labeler

This is a special purpose application that enables checking a DCO for a repository
and handling a label if the DCO passes or fails.

This was written for the helm charts repository.

This repository includes a Dockerfile and a Helm chart to install the application
within a Kubernetes cluster.

## Note On Helm Charts

This repository is meant to be under that helm org but is here as a temporary
dev location during a point in governance transition.

## TODO:

- [x] Write app
- [x] Chart
- [x] Write readme
- [x] Release
- [ ] Make more general purpose
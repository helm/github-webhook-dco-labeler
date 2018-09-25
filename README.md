# GitHub Webhook DCO Labeler

This is a utility special purpose application that enables checking a DCO for a repository
and handling a label if the DCO passes or fails. That is, if the DCO passes
a passing status check is added along with a label. If the DCO fails the status
check fails and the label is removed.

If you do not need the label handling consider using a service such as the
[one provided by probot](https://probot.github.io/apps/dco/)

This was written for the [helm charts repository](https://github.com/helm/charts).

This repository includes a Dockerfile and a Helm chart to install the application
within a Kubernetes cluster.
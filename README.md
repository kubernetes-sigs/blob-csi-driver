# blob-csi-driver helm chart repository

This branch hosts the Helm chart repository for blob-csi-driver via
GitHub Pages. Published automatically by the
`Publish Helm Chart to GitHub Pages` workflow.

To use:

    helm repo add blob-csi-driver https://kubernetes-sigs.github.io/blob-csi-driver
    helm repo update blob-csi-driver
    helm install blob-csi-driver blob-csi-driver/blob-csi-driver \
      --namespace kube-system --version <x.y.z>

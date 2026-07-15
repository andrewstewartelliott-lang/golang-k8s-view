## Usage

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

    helm repo add k8s-viewer https://andrewstewartelliott-lang.github.io/charts

If you had already added this repo earlier, run `helm repo update` to retrieve
the latest versions of the packages.  You can then run `helm search repo
{alias}` to see the charts.

To install the k8s-viewer chart:

    helm install k8s-viewer k8s-viewer/chart-golang-k8s-viewer

To uninstall the chart:

    helm delete demo
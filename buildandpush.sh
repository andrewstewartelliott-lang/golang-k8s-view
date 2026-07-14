go build .
docker build -t golang-k8s-view:latest .
docker tag golang-k8s-view:latest andrewstewartelliott/golang-k8s-view:latest
docker push andrewstewartelliott/golang-k8s-view:latest
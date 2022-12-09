```shell
# Install
$ ko apply -Rf config/

$ kubectl apply -f docs/registry/resources.yaml

# querying events and binding sink
$ go run cmd/kn/main.go eventtypes query -n discovery-demo \
  --cesql "type = 'dev.knative.sources.ping'" \
  --sink svc:event-display:discovery-demo \
  -oyaml --show triggers

# non existing events
$ go run cmd/kn/main.go eventtypes query -n discovery-demo \
  --cesql "type = 'dev.knative.sources.pingu'" \
  --sink svc:event-display:discovery-demo \
  -oyaml --show triggers
```
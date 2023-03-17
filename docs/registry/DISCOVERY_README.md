
## Generating triggers

```shell
# Consume events with type `dev.knative.sources.ping`
go run ./cmd/kn eventtypes query -n discovery-demo --cesql "type = 'dev.knative.sources.ping'" --sink svc:event-display:discovery-demo --show triggers -oyaml

# Consume events from source `/apis/v1/namespaces/discovery-demo/pingsources/ping-source-hello-world`
go run ./cmd/kn eventtypes query -n discovery-demo --cesql "source = '/apis/v1/namespaces/discovery-demo/pingsources/ping-source-hello-world'" --sink svc:event-display:discovery-demo --show triggers -oyaml
```

## Notes

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
  --cesql "type = 'dev.knative.sources.ping'" \
  --sink svc:event-display:discovery-demo \
  -oyaml --show triggers
```


## Generating triggers

```shell
# Consume events with type `dev.knative.sources.ping`
go run ./cmd/kn eventtypes query -n discovery-demo --cesql "type = 'dev.knative.sources.ping'" --sink svc:event-display:discovery-demo --show triggers -oyaml

# Consume events from source `/apis/v1/namespaces/discovery-demo/pingsources/ping-source-hello-world`
go run ./cmd/kn eventtypes query -n discovery-demo --cesql "source = '/apis/v1/namespaces/discovery-demo/pingsources/ping-source-hello-world'" --sink svc:event-display:discovery-demo --show triggers -oyaml
```

## Notes minimal

```shell
kubectl apply -f docs/registry/resources-minimal.yaml

kubectl run curl  --image=docker.io/curlimages/curl --rm=true --restart=Never -ti \
  -- -X POST -v \
  -H "content-type: application/json" \
  -H "ce-specversion: 1.0" \
  -H "ce-source: my/curl/command" \
  -H "ce-type: dev.knative.source.github.issues.opened" \
  -H "ce-id: 6cf17c7b-30b1-45a6-80b0-4cf58c92b947" \
  -H "ce-dataschema: https://raw.githubusercontent.com/octokit/webhooks/main/payload-schemas/api.github.com/issues/opened.schema.json" \
  -d '{"name":"Knative Discovery Demo"}' \
  http://broker-ingress.knative-eventing.svc.cluster.local/discovery-demo/discovery-broker
  
kubectl get eventtypes -n discovery-demo

go run ./cmd/kn eventtypes query -n discovery-demo --cesql "type = 'dev.knative.source.github.issues.opened'" --sink svc:event-display:discovery-demo --show triggers -oyaml

go run ./cmd/kn eventtypes query -n discovery-demo --cesql "type = 'dev.knative.source.github.issues.opened'" --sink svc:event-display:discovery-demo --show triggers -oyaml | kubectl apply -f -
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

This is an Azure Function app that provides the ability to create a new Argo CD gateway in Octopus and define some applications. It is used to mock deployments, as the gateway provides all the details required to make git commits, without requiring an actual Argo CD instance.

This is an example curl request:

```bash
curl -X POST https://argocd-gateway-mock-gecvabhfc9fagzan.eastus-01.azurewebsites.net/api/applications \
  -H "Content-Type: application/json" \
  -d '{
    "server_api_url": "https://instance.octopus.app",
    "server_access_token": "access token goes here",
    "space_id": "Spaces-5751",
    "gateway_name": "Mocked Argo CD Instance",
    "environments": ["production", "development"],
    "tenants": [],
    "applications": [
      {
        "name": "octopub-manifest-parent-development",
        "project": "default",
        "namespace": "argocd",
        "destination_server": "https://kubernetes.default.svc",
        "destination_namespace": "octopub-manifest-parent-development",
        "repo_url": "https://mockgit.octopusdemos.com/repo/argocd",
        "path": "octopub-manifest/application/development",
        "sync_status": "Synced",
        "health_status": "Healthy",
        "octopus_project": "argo-cd-octopub-manifest",
        "octopus_environment": "development"
      }
    ]
  }'
```

Health check:

```aiignore
curl https://argocd-gateway-mock-gecvabhfc9fagzan.eastus-01.azurewebsites.net/api/health
```
# deploy/k8s

Kubernetes manifests for unimatrix. Apply order:

```bash
kubectl apply -f namespace.yaml

# Provide the session-key secret (32+ bytes, never commit it):
kubectl -n unimatrix create secret generic unimatrix-secrets \
  --from-literal=session_key="$(openssl rand -hex 32)"

kubectl apply -f pvc.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```

Roll a new image (this is what devops-drone does after merge):

```bash
kubectl -n unimatrix set image deployment/unimatrix \
  unimatrix=ghcr.io/florianwenzel/unimatrix:sha-<short>
kubectl -n unimatrix rollout status deployment/unimatrix --timeout=2m
```

Rollback:

```bash
kubectl -n unimatrix rollout undo deployment/unimatrix
```

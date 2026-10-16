#!/usr/bin/env bash
# Turn on API audit logging in the kind lab cluster (for the translation
# check, C-17), the way safe-ops-agent-k8s does: copy the policy onto the
# control-plane node and add the flags to the API server's static pod
# manifest. Idempotent; the kubelet restarts the API server when the
# manifest changes. The log is /var/log/kubernetes/audit/audit.log on the
# node (read it with docker exec).
set -euo pipefail
cd "$(dirname "$0")/.."
node=planproof-control-plane
manifest=/etc/kubernetes/manifests/kube-apiserver.yaml

docker exec "$node" mkdir -p /etc/kubernetes/audit /var/log/kubernetes/audit
docker cp cluster/audit-policy.yaml "$node":/etc/kubernetes/audit/policy.yaml
hash=$(sha256sum cluster/audit-policy.yaml | cut -c1-16)

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
docker cp "$node":"$manifest" "$tmp/in.yaml"
python3 scripts/audit_apiserver.py "$hash" < "$tmp/in.yaml" > "$tmp/out.yaml"
if cmp -s "$tmp/in.yaml" "$tmp/out.yaml"; then
  echo "audit: already on"
  exit 0
fi
docker cp "$tmp/out.yaml" "$node":"$manifest"
echo "audit: API server restarting with the audit policy"
kubectl="kubectl --kubeconfig .kube/planproof --context kind-planproof"
sleep 5
for i in $(seq 1 60); do
  if $kubectl get --raw /readyz >/dev/null 2>&1 && docker exec "$node" test -s /var/log/kubernetes/audit/audit.log -o -e /var/log/kubernetes/audit/audit.log; then
    echo "audit: on"; exit 0
  fi
  sleep 2
done
echo "audit: the API server didn't come back" >&2; exit 1

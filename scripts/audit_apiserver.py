"""Add the audit flags and mounts to the API server's static pod manifest.

    python3 audit_apiserver.py POLICY_HASH < kube-apiserver.yaml > new.yaml

Idempotent: the output is the input unchanged if everything is already
there. POLICY_HASH goes in an annotation, so a changed policy changes the
manifest and the kubelet restarts the API server, which reads the policy
only at start. Adapted from safe-ops-agent-k8s.
"""
import sys

import yaml

FLAGS = {
    "--audit-policy-file": "/etc/kubernetes/audit/policy.yaml",
    "--audit-log-path": "/var/log/kubernetes/audit/audit.log",
    "--audit-log-format": "json",
    "--audit-log-maxsize": "100",
    "--audit-log-maxbackup": "3",
}
MOUNTS = [
    ("audit-config", "/etc/kubernetes/audit", True, "Directory"),
    ("audit-log", "/var/log/kubernetes/audit", False, "DirectoryOrCreate"),
]


def main():
    src = sys.stdin.read()
    pod = yaml.safe_load(src)
    before = yaml.safe_dump(pod, sort_keys=False)
    c = pod["spec"]["containers"][0]
    c["command"] = [a for a in c["command"] if a.split("=", 1)[0] not in FLAGS] + [f"{k}={v}" for k, v in FLAGS.items()]
    names = {n for n, *_ in MOUNTS}
    c["volumeMounts"] = [m for m in c.get("volumeMounts", []) if m["name"] not in names] + [
        {"mountPath": p, "name": n, "readOnly": ro} for n, p, ro, _ in MOUNTS]
    pod["spec"]["volumes"] = [v for v in pod["spec"].get("volumes", []) if v["name"] not in names] + [
        {"hostPath": {"path": p, "type": k}, "name": n} for n, p, _, k in MOUNTS]
    pod["metadata"].setdefault("annotations", {})["planproof.io/audit-policy"] = sys.argv[1]
    after = yaml.safe_dump(pod, sort_keys=False)
    sys.stdout.write(src if after == before else after)


if __name__ == "__main__":
    main()

#!/usr/bin/env bash
# End to end: build the image, stand up the kind environment with Terraform,
# then prove the app works and is observed the way the SLOs assume.
#
#   scripts/e2e.sh          # build, apply, check
#   scripts/e2e.sh destroy  # tear the cluster down
#
# Needs docker, kind, kubectl, helm and terraform on PATH.
# DOCKER_BUILD_FLAGS is passed to docker build (e.g. --network host).
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
env_dir=$root/terraform/envs/kind
image=${IMAGE:-}
network_policy=${NETWORK_POLICY:-true}
tf() { terraform -chdir="$env_dir" "$@"; }
step() { printf '\n== %s\n' "$*"; }

if [[ ${1:-} == destroy ]]; then
  tf destroy -auto-approve -input=false
  exit 0
fi

step "build"
docker build ${DOCKER_BUILD_FLAGS:-} -t hop:dev "$root"
if [[ -z $image ]]; then
  # Tag by content, so a code change is a new tag and the pods roll.
  image=hop:$(docker image inspect --format '{{.Id}}' hop:dev | cut -c8-19)
  docker tag hop:dev "$image"
fi
echo "image: $image"

step "terraform apply (kind cluster, platform, hop)"
tf init -input=false
tf apply -auto-approve -input=false -var "image=$image" -var "network_policy=$network_policy"

KUBECONFIG=$(tf output -raw kubeconfig_path)
export KUBECONFIG
kubectl get nodes -o wide

probe_pod() { # name, shell command
  kubectl -n hop apply -f - >/dev/null <<YAML
apiVersion: v1
kind: Pod
metadata: {name: $1}
spec:
  restartPolicy: Never
  securityContext: {runAsNonRoot: true, runAsUser: 65532, seccompProfile: {type: RuntimeDefault}}
  containers:
    - name: probe
      image: curlimages/curl:8.17.0
      securityContext: {allowPrivilegeEscalation: false, capabilities: {drop: [ALL]}}
      command: [sh, -c]
      args:
        - |
          $2
YAML
  kubectl -n hop wait --for=condition=Ready "pod/$1" --timeout=60s >/dev/null 2>&1 || true
}
pod_result() { # name -> waits for the pod to finish, prints its logs, succeeds if it did
  local phase=""
  for _ in $(seq 1 180); do
    phase=$(kubectl -n hop get "pod/$1" -o jsonpath='{.status.phase}')
    [[ $phase == Succeeded || $phase == Failed ]] && break
    sleep 1
  done
  kubectl -n hop logs "pod/$1"
  kubectl -n hop delete "pod/$1" --wait=false >/dev/null
  [[ $phase == Succeeded ]]
}

step "helm test: create a link through the Service and follow it"
helm test hop -n hop --logs

pf_pids=()
cleanup() { kill "${pf_pids[@]}" 2>/dev/null || true; }
trap cleanup EXIT
port_forward() { # namespace service local:remote
  kubectl -n "$1" port-forward "svc/$2" "$3" >/dev/null 2>&1 &
  pf_pids+=($!)
}
port_forward hop hop 18080:80
port_forward monitoring kube-prometheus-stack-prometheus 19090:9090
sleep 3

prom() { curl -fsS --get "http://127.0.0.1:19090/api/v1/query" --data-urlencode "query=$1"; }
value() { prom "$1" | python3 -c 'import json,sys; r=json.load(sys.stdin)["data"]["result"]; print(r[0]["value"][1] if r else "none")'; }
wait_for() { # description, query, python condition on v
  local desc=$1 query=$2 cond=$3 v
  for _ in $(seq 1 60); do
    v=$(value "$query")
    if python3 -c "v='$v'; import sys; sys.exit(0 if v!='none' and ($cond) else 1)"; then
      echo "ok: $desc ($v)"
      return 0
    fi
    sleep 5
  done
  echo "FAIL: $desc (last value: $v)" >&2
  return 1
}

step "traffic: 200 redirects and 50 misses"
code=$(curl -fsS -X POST localhost:18080/api/links -d '{"url":"https://example.com/e2e"}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["code"])')
for _ in $(seq 1 200); do curl -s -o /dev/null "localhost:18080/$code"; done
for _ in $(seq 1 50); do curl -s -o /dev/null "localhost:18080/missing"; done
hits=$(curl -fsS "localhost:18080/api/links/$code" | python3 -c 'import json,sys; print(json.load(sys.stdin)["hits"])')
[[ $hits == 200 ]] || { echo "FAIL: expected 200 hits, got $hits" >&2; exit 1; }
echo "ok: 200 hits counted in Postgres"

step "observability (with steady background traffic, so the 5m rates are not 0/0)"
( while true; do curl -s -o /dev/null "localhost:18080/$code"; sleep 0.2; done ) &
pf_pids+=($!)
wait_for "both hop pods scraped" 'sum(up{job="hop"})' 'float(v) >= 2'
wait_for "redirects counted" 'sum(hop_http_requests_total{job="hop",route="GET /{code}",code="302"})' 'float(v) >= 200'
wait_for "SLO rules loaded and evaluating" 'slo:hop_availability_errors:ratio_rate5m' 'float(v) == 0'
wait_for "latency SLI recorded" 'slo:hop_latency_errors:ratio_rate5m' 'float(v) < 0.01'
alerts=$(value 'count(ALERTS{alertname=~"Hop.*", alertstate="firing"})')
[[ $alerts == none ]] || { echo "FAIL: hop alerts firing: $alerts" >&2; exit 1; }
echo "ok: no hop alerts firing"

step "rolling restart under load loses no requests"
# Runs inside the cluster: a port-forward is pinned to one pod and breaks
# when that pod is replaced, so it cannot measure a rollout.

probe_pod rollout-probe "for i in \$(seq 1 600); do curl -s -o /dev/null -w '%{http_code}\\n' http://hop/$code; sleep 0.05; done | sort | uniq -c"
kubectl -n hop rollout restart deploy/hop
kubectl -n hop rollout status deploy/hop --timeout=180s
out=$(pod_result rollout-probe)
echo "$out"
if echo "$out" | grep -vE '^ *[0-9]+ 302$' | grep -q '[0-9]'; then
  echo "FAIL: requests failed during the rollout" >&2
  exit 1
fi
echo "ok: all 600 requests during the rollout were redirects"

if [[ $network_policy == true ]]; then
step "network policy: metrics are reachable only from the monitoring namespace"
probe_pod np-probe "curl -sS -m 5 -o /dev/null http://hop:9090/metrics"
if pod_result np-probe >/dev/null 2>&1; then
  echo "FAIL: metrics reachable from the hop namespace" >&2
  exit 1
fi
echo "ok: metrics blocked outside monitoring"
fi

step "all checks passed"

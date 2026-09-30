#!/usr/bin/env bash
# 用调用图检查结构迁移会不会绕回 ensureSourceStationStorage。
# 只看本模块里的调用。标准库会把无关函数粘成一个大连通块，那些边不参与判断。
# 只在仓库里跑，不打进发布包。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND="${ROOT}/backend"
export PATH="$(go env GOPATH)/bin:${PATH}"
export GOFLAGS="${GOFLAGS:-}"

CALLGRAPH_VERSION="${CALLGRAPH_VERSION:-v0.25.1}"

if ! command -v callgraph >/dev/null 2>&1; then
  go install "golang.org/x/tools/cmd/callgraph@${CALLGRAPH_VERSION}"
fi

edges="$(mktemp)"
trap 'rm -f "$edges"' EXIT

echo "== 调用图（vta）=="
(
  cd "$BACKEND"
  callgraph -algo=vta -format='{{.Caller}} --> {{.Callee}}' .
) >"$edges"

python3 - "$edges" "$BACKEND/handler/source_station_migrate.go" <<'PY'
import collections
import re
import sys

edges_path, migrate_path = sys.argv[1], sys.argv[2]

# 树遍历的自递归是合法的。迁移函数不在这份名单里。
whitelist = {
    "buildMenuTree",
    "buildManageTree",
    "copyOnlineUpdatePath",
    "findTencentRealnameProductItems",
    "collectTencentRealnameScopes",
}

watched_fixed = {
    "ensureSourceStationStorage",
    "ensureSourceStationMigrations",
    "runSourceStationMigration",
}

text = open(migrate_path, encoding="utf-8").read()
start = text.find("steps := []struct")
end = text.find("for _, step := range steps", start)
if start < 0 or end < 0:
    sys.exit("没有找到结构迁移步骤列表")
steps = re.findall(r",\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}", text[start:end])
if "migrateAppRepoBindings" not in steps:
    sys.exit("迁移步骤里没有 migrateAppRepoBindings")
watched = watched_fixed | set(steps)

def in_app(node: str) -> bool:
    return node.startswith("auto_pro/") or node.startswith("(*auto_pro/")

def base_name(node: str) -> str:
    node = node.strip()
    if "." not in node:
        return node
    return node.rsplit(".", 1)[1]

graph = collections.defaultdict(set)
nodes = set()
with open(edges_path, encoding="utf-8", errors="replace") as handle:
    for raw in handle:
        line = raw.strip()
        if not line or " --> " not in line:
            continue
        caller, callee = line.split(" --> ", 1)
        caller, callee = caller.strip(), callee.strip()
        if not caller or not callee or not in_app(caller) or not in_app(callee):
            continue
        # 白名单里的函数调用自己，不参与成环判断。
        if caller == callee and base_name(caller) in whitelist:
            nodes.add(caller)
            continue
        graph[caller].add(callee)
        nodes.add(caller)
        nodes.add(callee)

by_base = collections.defaultdict(list)
for node in nodes:
    by_base[base_name(node)].append(node)

# 步骤函数通过函数值调用，调用图不一定能把这一跳标成静态边。按步骤表补上。
runners = by_base.get("runSourceStationMigration") or ["auto_pro/handler.runSourceStationMigration"]
for runner in runners:
    nodes.add(runner)
    for step in steps:
        targets = by_base.get(step) or [f"auto_pro/handler.{step}"]
        for target in targets:
            nodes.add(target)
            graph[runner].add(target)

index = {node: i for i, node in enumerate(sorted(nodes))}
rev = collections.defaultdict(set)
for caller, callees in graph.items():
    for callee in callees:
        rev[callee].add(caller)

visited = set()
order = []

def dfs(node):
    visited.add(node)
    for nxt in graph.get(node, ()):
        if nxt not in visited:
            dfs(nxt)
    order.append(node)

for node in list(nodes):
    if node not in visited:
        dfs(node)

visited.clear()
components = []

def rdfs(node, bucket):
    visited.add(node)
    bucket.append(node)
    for nxt in rev.get(node, ()):
        if nxt not in visited:
            rdfs(nxt, bucket)

for node in reversed(order):
    if node not in visited:
        bucket = []
        rdfs(node, bucket)
        components.append(bucket)

self_nodes = {caller for caller, callees in graph.items() if caller in callees}
failed = False
for bucket in components:
    names = {base_name(node) for node in bucket}
    interesting = names & watched
    if not interesting:
        continue
    cyclic = len(bucket) > 1 or any(node in self_nodes for node in bucket)
    if not cyclic:
        continue
    failed = True
    shown = sorted(names)
    print("结构迁移调用成环，涉及: " + "、".join(sorted(interesting)), file=sys.stderr)
    print("强连通分量: " + "、".join(shown), file=sys.stderr)

if failed:
    sys.exit(1)
print("结构迁移调用图没有环")
PY

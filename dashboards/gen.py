#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-2.0-only
"""Generate the example Grafana and Perses dashboards from one panel table.

    python3 dashboards/gen.py

Edit PANELS/SECTIONS and re-run; do not hand-edit the generated JSON. Both use the
variables job (default .*mattx.*), instance, and (Grafana) datasource. Only metrics the
exporter emits may be used; internal/dashboards tests enforce that.
"""
import json
import pathlib

HERE = pathlib.Path(__file__).resolve().parent
S = 'job=~"$job",instance=~"$instance"'

# id -> (title, description, [(expr, legend)], options)  options: unit, bar, table
PANELS = {
    "nodes_table": ("Cluster nodes", "Each node as seen from the selected instance(s): load, free memory, affinity and presence (/proc/mattx/nodes).",
        [(f"mattx_node_load{{{S}}}", "load"), (f"mattx_node_mem_free_bytes{{{S}}}", "mem free"),
         (f"mattx_node_affinity{{{S}}}", "affinity"), (f"mattx_node_up{{{S}}}", "up")], {"table": True}),
    "node_load": ("Node load", "mattx_node_load: CPU load of each node as reported in /proc/mattx/nodes.",
        [(f"mattx_node_load{{{S}}}", "{{instance}} sees node {{node}}")], {}),
    "node_mem": ("Node free memory", "mattx_node_mem_free_bytes per node.",
        [(f"mattx_node_mem_free_bytes{{{S}}}", "{{instance}} sees node {{node}}")], {"unit": "bytes"}),
    "node_aff": ("Node affinity", "mattx_node_affinity: scheduling weight of each node.",
        [(f"mattx_node_affinity{{{S}}}", "{{instance}} sees node {{node}}")], {}),
    "node_up": ("Node up", "1 if the node is in the cluster table, 0 if it was seen before and is gone.",
        [(f"mattx_node_up{{{S}}}", "{{instance}} sees node {{node}}")], {}),
    "moves": ("Migrations", "Migration transitions in the interval by direction (out, hop, return, exit: own processes; in, departed: hosted guests) and process name.",
        [(f"sum by (instance, direction, comm) (increase(mattx_migrations_total{{{S}}}[$__rate_interval])) > 0", "{{instance}} {{direction}} {{comm}}")], {"bar": True}),
    "guests": ("Guests hosted", "Processes of other nodes currently hosted here.",
        [(f"mattx_guests_hosted{{{S}}}", "{{instance}}")], {}),
    "remote": ("Own processes running elsewhere", "Processes started here that currently run on other nodes.",
        [(f"mattx_remote_processes{{{S}}}", "{{instance}}")], {}),
    "where": ("Where own processes run", "mattx_remote_process by process name and target node.",
        [(f"sum by (instance, comm, target_node) (mattx_remote_process{{{S}}})", "{{instance}}: {{comm}} -> node {{target_node}}")], {}),
    "guestproc": ("Hosted guests by home node", "mattx_guest_process by process name and the node they come from.",
        [(f"sum by (instance, comm, home_node) (mattx_guest_process{{{S}}})", "{{instance}}: {{comm}} <- node {{home_node}}")], {}),
    "pidmig": ("Migrations per pid (needs per_pid_metrics: true)", "mattx_pid_migrations_total only exists when the exporter runs with per_pid_metrics: true in /etc/mattx_exporter.yml (off by default because of cardinality). Empty otherwise.",
        [(f"topk(10, mattx_pid_migrations_total{{{S}}})", "{{instance}} pid {{pid}} {{comm}}")], {}),
    "service": ("mattx service active", "1 if the mattx systemd service is active.",
        [(f"mattx_service_active{{{S}}}", "{{instance}}")], {}),
    "kdump": ("kdump directories", "Crash dump directories in the kdump directory.",
        [(f"mattx_kdump_dirs{{{S}}}", "{{instance}}")], {}),
    "modloads": ("Module loads since boot", "Times mattx.ko was loaded since boot.",
        [(f"mattx_module_loads_since_boot{{{S}}}", "{{instance}}")], {}),
    "execmem": ("execmem errors since boot", "Kernel 'execmem: unable to allocate' lines since boot.",
        [(f"mattx_execmem_errors_since_boot{{{S}}}", "{{instance}}")], {}),
    "up": ("Exporter up", "Prometheus scrape success of the exporter.",
        [(f"up{{{S}}}", "{{instance}}")], {}),
    "scrape_err": ("Exporter scrape errors (5m)", "Errors while collecting, by collector.",
        [(f"increase(mattx_exporter_scrape_errors_total{{{S}}}[5m])", "{{instance}} {{collector}}")], {}),
    "build": ("Exporter build", "Version, commit and Go version of each exporter.",
        [(f"mattx_build_info{{{S}}}", "build")], {"table": True}),
}

# (title, [(panel, width, height)]) ; 24 columns
SECTIONS = [
    ("Cluster nodes", [("nodes_table", 24, 8), ("node_load", 12, 7), ("node_mem", 12, 7), ("node_aff", 12, 7), ("node_up", 12, 7)]),
    ("Migrations", [("moves", 24, 7), ("guests", 12, 7), ("remote", 12, 7), ("where", 12, 7), ("guestproc", 12, 7), ("pidmig", 24, 7)]),
    ("Host health", [("service", 12, 6), ("kdump", 12, 6), ("modloads", 12, 6), ("execmem", 12, 6)]),
    ("Exporter health", [("up", 12, 6), ("scrape_err", 12, 6), ("build", 24, 5)]),
]
TITLE = "MattX exporter"
DESC = "Cluster nodes, process migration, host health and exporter health from mattx_exporter. Variables: job (regex, default .*mattx.*), instance."


def layout(items):
    x = y = rowh = 0
    out = []
    for pid, w, h in items:
        if x + w > 24:
            x, y, rowh = 0, y + rowh, 0
        out.append((pid, x, y, w, h))
        x, rowh = x + w, max(rowh, h)
    return out


def perses():
    panels, layouts = {}, []
    for pid, (title, desc, queries, o) in PANELS.items():
        if o.get("table"):
            plugin = {"kind": "Table", "spec": {"transforms": [{"kind": "JoinByColumnValue", "spec": {"columns": ["node" if pid == "nodes_table" else "instance"]}}]}}
        else:
            spec = {}
            if "unit" in o:
                spec["yAxis"] = {"format": {"unit": o["unit"]}}
            if o.get("bar"):
                spec["visual"] = {"display": "bar"}
            plugin = {"kind": "TimeSeriesChart", "spec": spec}
        panels[pid] = {"kind": "Panel", "spec": {
            "display": {"name": title, "description": desc}, "plugin": plugin,
            "queries": [{"kind": "TimeSeriesQuery", "spec": {"plugin": {
                "kind": "PrometheusTimeSeriesQuery", "spec": {"query": e, "seriesNameFormat": lg}}}} for e, lg in queries]}}
    for title, items in SECTIONS:
        layouts.append({"kind": "Grid", "spec": {"display": {"title": title}, "items": [
            {"x": x, "y": y, "width": w, "height": h, "content": {"$ref": f"#/spec/panels/{pid}"}}
            for pid, x, y, w, h in layout(items)]}})
    lv = lambda name, label, matcher: {"kind": "ListVariable", "spec": {
        "name": name, "display": {"name": label}, "allowMultiple": True, "allowAllValue": True,
        "defaultValue": "$__all",
        "plugin": {"kind": "PrometheusLabelValuesVariable", "spec": {"labelName": name, "matchers": [matcher]}}}}
    return {
        "kind": "Dashboard",
        "metadata": {"name": "mattx-exporter", "project": "default"},
        "spec": {
            "display": {"name": TITLE, "description": DESC},
            "duration": "1h", "refreshInterval": "30s",
            "variables": [
                {"kind": "TextVariable", "spec": {"name": "job", "display": {"name": "Job (regex)"}, "value": ".*mattx.*"}},
                lv("instance", "Instance", 'mattx_build_info{job=~"$job"}'),
            ],
            "panels": panels, "layouts": layouts,
        },
    }


def grafana():
    ds = {"type": "prometheus", "uid": "${datasource}"}
    panels, n, y = [], 0, 0
    for title, items in SECTIONS:
        n += 1
        panels.append({"id": n, "type": "row", "title": title, "collapsed": False,
                       "gridPos": {"h": 1, "w": 24, "x": 0, "y": y}, "panels": []})
        y += 1
        base = y
        end = y
        for pid, x, ly, w, h in layout(items):
            t, desc, queries, o = PANELS[pid]
            n += 1
            gp = {"h": h, "w": w, "x": x, "y": base + ly}
            end = max(end, base + ly + h)
            targets = [{"refId": chr(65 + i), "datasource": ds, "expr": e, "legendFormat": lg,
                        "editorMode": "code", "range": not o.get("table"), "instant": bool(o.get("table")),
                        **({"format": "table"} if o.get("table") else {})} for i, (e, lg) in enumerate(queries)]
            if o.get("table"):
                key = "node" if pid == "nodes_table" else "instance"
                panels.append({"id": n, "type": "table", "title": t, "description": desc, "datasource": ds, "gridPos": gp,
                               "targets": targets, "fieldConfig": {"defaults": {}, "overrides": []}, "options": {},
                               "transformations": [{"id": "merge", "options": {}}]})
            else:
                defaults = {"custom": {"drawStyle": "bars" if o.get("bar") else "line",
                                       "fillOpacity": 60 if o.get("bar") else 10, "showPoints": "never"}}
                if "unit" in o:
                    defaults["unit"] = o["unit"]
                panels.append({"id": n, "type": "timeseries", "title": t, "description": desc, "datasource": ds, "gridPos": gp,
                               "fieldConfig": {"defaults": defaults, "overrides": []},
                               "options": {"legend": {"displayMode": "list", "placement": "bottom"}, "tooltip": {"mode": "multi", "sort": "desc"}},
                               "targets": targets})
        y = end
    q = lambda name, label, query, multi: {
        "name": name, "label": label, "type": "query", "datasource": ds,
        "query": {"query": query, "refId": name}, "definition": query,
        "multi": multi, "includeAll": multi, "allValue": ".*" if multi else None,
        "current": {"selected": True, "text": "All", "value": "$__all"}, "refresh": 2, "sort": 1}
    inst = q("instance", "Instance", 'label_values(mattx_build_info{job=~"$job"}, instance)', True)
    return {
        "uid": "mattx-exporter", "title": TITLE, "description": DESC, "schemaVersion": 39, "version": 1,
        "editable": True, "refresh": "30s", "time": {"from": "now-1h", "to": "now"}, "tags": ["mattx"],
        "panels": panels,
        "templating": {"list": [
            {"name": "datasource", "label": "Data source", "type": "datasource", "query": "prometheus",
             "current": {}, "refresh": 1, "hide": 0},
            {"name": "job", "label": "Job (regex)", "type": "textbox", "query": ".*mattx.*",
             "current": {"text": ".*mattx.*", "value": ".*mattx.*"}, "hide": 0},
            inst]},
    }


def write(path, doc):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(doc, indent=2) + "\n")
    print("wrote", path.relative_to(HERE.parent))


if __name__ == "__main__":
    write(HERE / "perses/mattx-exporter.json", perses())
    write(HERE / "grafana/mattx-exporter.json", grafana())

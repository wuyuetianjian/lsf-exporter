#!/bin/sh
# Emit exporter extension JSON only. Send diagnostics to stderr.

python3 - <<'PY'
import json
import re
import shutil
import subprocess
import sys


def run(command):
    executable = command[0]
    if shutil.which(executable) is None:
        print(f"{executable} not found in PATH", file=sys.stderr)
        return []
    try:
        output = subprocess.check_output(command, text=True, stderr=subprocess.PIPE)
        return [line.strip() for line in output.splitlines() if line.strip()]
    except subprocess.CalledProcessError as exc:
        detail = exc.stderr.strip() if exc.stderr else str(exc)
        print(f"{' '.join(command)} failed: {detail}", file=sys.stderr)
        return []


def to_int(value):
    value = str(value).strip()
    if value in ("", "-", "unlimited", "UNLIMITED"):
        return 0
    try:
        return int(float(value))
    except ValueError:
        return 0


def to_float(value):
    value = str(value).strip()
    if value in ("", "-"):
        return 0.0
    try:
        return float(value)
    except ValueError:
        return 0.0


def bool_from_status(status, keyword):
    return keyword.lower() in status.lower()


def compact_json(value):
    return json.dumps(value, separators=(",", ":"), sort_keys=True)


def normalize_group_name(value):
    return value.strip().strip("():").lstrip("@")


def normalize_queue_group_name(value):
    return normalize_group_name(value).rstrip("/")


def split_host_tokens(value):
    value = value.replace("(", " ").replace(")", " ").replace(",", " ")
    return [part.strip() for part in value.split() if part.strip()]


cluster = {"status": "unknown"}
lsid_lines = run(["lsid"])
for line in lsid_lines:
    lower = line.lower()
    match = re.search(r"<([^>]+)>", line)
    value = match.group(1).strip() if match else ""
    if not value:
        tail = re.split(r"\bis\b|:", line, maxsplit=1, flags=re.IGNORECASE)
        value = tail[1].strip().strip(".") if len(tail) > 1 else ""

    if "cluster" in lower and "name" in lower and value:
        cluster["name"] = value
    elif "master" in lower and "name" in lower and value:
        cluster["master"] = value
        cluster["master_up"] = True

if lsid_lines:
    cluster.setdefault("status", "ok")
    cluster.setdefault("raw", {})["lsid"] = " | ".join(lsid_lines)
else:
    cluster["master_up"] = False


host_groups = {}
current_group = None
for line in run(["bmgroup", "-w"]):
    lower = line.lower()
    if lower.startswith("group_name") or lower.startswith("-"):
        continue
    parts = line.split()
    if not parts:
        continue
    first = normalize_group_name(parts[0])
    if len(parts) > 1:
        current_group = first
        member_text = " ".join(parts[1:])
    elif current_group:
        member_text = " ".join(parts)
    else:
        continue
    members = host_groups.setdefault(current_group, [])
    members.extend(
        token
        for token in split_host_tokens(member_text)
        if token.lower() not in ("all", "-")
    )

for group_name, members in list(host_groups.items()):
    unique_members = sorted(set(members))
    host_groups[group_name] = unique_members


user_groups = {}
current_group = None
for line in run(["bugroup", "-w"]):
    lower = line.lower()
    if lower.startswith("group_name") or lower.startswith("-"):
        continue
    parts = line.split()
    if not parts:
        continue
    first = normalize_group_name(parts[0])
    if len(parts) > 1:
        current_group = first
        member_text = " ".join(parts[1:])
    elif current_group:
        member_text = " ".join(parts)
    else:
        continue
    members = user_groups.setdefault(current_group, [])
    members.extend(
        token
        for token in split_host_tokens(member_text)
        if token.lower() not in ("all", "-")
    )

for group_name, members in list(user_groups.items()):
    unique_members = sorted(set(members))
    user_groups[group_name] = unique_members


hosts = []
all_host_names = []
host_cmd = [
    "bhosts",
    "-o",
    "host_name status max njobs run ssusp ususp",
    "-noheader",
]
for line in run(host_cmd):
    parts = line.split()
    if len(parts) < 7:
        print(f"skip unparsable bhosts line: {line}", file=sys.stderr)
        continue

    name, status, max_jobs, num_jobs, running, ssusp, ususp = parts[:7]
    groups = sorted(
        group_name
        for group_name, members in host_groups.items()
        if name in members
    )
    all_host_names.append(name)
    hosts.append(
        {
            "name": name,
            "status": status,
            "closed": status.lower() not in ("ok", "open"),
            "max_jobs": to_int(max_jobs),
            "num_jobs": to_int(num_jobs),
            "running": to_int(running),
            "suspended": to_int(ssusp) + to_int(ususp),
            "resources": {
                "host_groups": ",".join(groups),
            } if groups else {},
            "raw": {
                "host_groups": compact_json(groups),
            } if groups else {},
        }
    )


all_user_names = []
for line in run(["busers", "-w"]):
    lower = line.lower()
    if lower.startswith("user/group") or lower.startswith("-"):
        continue
    parts = line.split()
    if not parts:
        continue
    name = parts[0].strip()
    if name and name.lower() != "all" and "/" not in name:
        all_user_names.append(name)
all_user_names = sorted(set(all_user_names))


queue_host_specs = {}
queue_user_specs = {}
queue_detail_lines = {}
current_queue = None
collecting_field = None
for line in run(["bqueues", "-l"]):
    stripped = line.strip()
    if stripped.startswith("QUEUE:"):
        current_queue = stripped.split(":", 1)[1].strip().split()[0]
        queue_detail_lines[current_queue] = [line]
        collecting_field = None
        continue
    if current_queue:
        queue_detail_lines.setdefault(current_queue, []).append(line)
    if current_queue and stripped.startswith("HOSTS:"):
        queue_host_specs[current_queue] = stripped.split(":", 1)[1].strip()
        collecting_field = "hosts"
        continue
    if current_queue and stripped.startswith("USERS:"):
        queue_user_specs[current_queue] = stripped.split(":", 1)[1].strip()
        collecting_field = "users"
        continue
    if current_queue and collecting_field:
        if re.match(r"^[A-Z][A-Z0-9_ ]*:", stripped):
            collecting_field = None
            continue
        if collecting_field == "hosts":
            queue_host_specs[current_queue] = (
                queue_host_specs.get(current_queue, "") + " " + line
            ).strip()
        elif collecting_field == "users":
            queue_user_specs[current_queue] = (
                queue_user_specs.get(current_queue, "") + " " + line
            ).strip()


host_group_members_cache = {}
user_group_members_cache = {}


def resolve_host_group_members(group_name):
    clean_group = normalize_queue_group_name(group_name)
    if clean_group in host_group_members_cache:
        return host_group_members_cache[clean_group]

    members = []
    for line in run(["bhosts", "-o", "host_name", "-noheader", clean_group]):
        parts = line.split()
        if parts:
            host = parts[0].strip()
            if host and host != clean_group:
                members.append(host)
    members = sorted(set(members))
    host_group_members_cache[clean_group] = members
    return members


def resolve_queue_hosts(host_spec):
    tokens = split_host_tokens(host_spec)
    if not tokens:
        return [], []
    if any(token.lower() == "all" for token in tokens):
        return sorted(all_host_names), []

    resolved_hosts = set()
    resolved_groups = set()
    for token in tokens:
        if "/" in token:
            group_name = normalize_queue_group_name(token)
            resolved_groups.add(group_name)
            resolved_hosts.update(resolve_host_group_members(group_name))
        elif token not in ("-", ""):
            resolved_hosts.add(token)
    return sorted(resolved_hosts), sorted(resolved_groups)


def resolve_user_group_members(group_name):
    clean_group = normalize_queue_group_name(group_name)
    if clean_group in user_group_members_cache:
        return user_group_members_cache[clean_group]

    members = user_groups.get(clean_group, [])
    if not members:
        for line in run(["bugroup", "-w", clean_group]):
            lower = line.lower()
            if lower.startswith("group_name") or lower.startswith("-"):
                continue
            parts = line.split()
            if len(parts) > 1:
                members.extend(
                    token
                    for token in split_host_tokens(" ".join(parts[1:]))
                    if token.lower() not in ("all", "-")
                )
    members = sorted(set(members))
    user_group_members_cache[clean_group] = members
    return members


def resolve_queue_users(user_spec):
    tokens = split_host_tokens(user_spec)
    if not tokens:
        return [], []
    if any(token.lower() == "all" for token in tokens):
        return sorted(all_user_names), []

    resolved_users = set()
    resolved_groups = set()
    for token in tokens:
        if "/" in token:
            group_name = normalize_queue_group_name(token)
            resolved_groups.add(group_name)
            resolved_users.update(resolve_user_group_members(group_name))
        elif token not in ("-", ""):
            resolved_users.add(token)
    return sorted(resolved_users), sorted(resolved_groups)


def queue_interactive(queue_name):
    interactive_line = ""
    for line in queue_detail_lines.get(queue_name, []):
        if re.search(r"\bNO_INTERACTIVE\b", line, re.IGNORECASE):
            return False, line
        if re.search(r"\bINTERACTIVE\b", line, re.IGNORECASE):
            interactive_line = line
    if interactive_line:
        return True, interactive_line
    return False, ""


queues = []
queue_cmd = [
    "bqueues",
    "-o",
    "queue_name status priority njobs pend run ssusp ususp max",
    "-noheader",
]
for line in run(queue_cmd):
    parts = line.split()
    if len(parts) < 8:
        print(f"skip unparsable bqueues line: {line}", file=sys.stderr)
        continue

    name, status, priority, num_jobs, pending, running, ssusp, ususp = parts[:8]
    max_jobs = parts[8] if len(parts) > 8 else "0"
    host_spec = queue_host_specs.get(name, "")
    user_spec = queue_user_specs.get(name, "")
    queue_hosts, queue_groups = resolve_queue_hosts(host_spec)
    queue_users, queue_user_groups = resolve_queue_users(user_spec)
    interactive, interactive_source = queue_interactive(name)
    queue = {
        "name": name,
        "status": status,
        "priority": to_int(priority),
        "open": bool_from_status(status, "open"),
        "active": bool_from_status(status, "active"),
        "max_jobs": to_int(max_jobs),
        "num_jobs": to_int(num_jobs),
        "pending": to_int(pending),
        "running": to_int(running),
        "suspended": to_int(ssusp) + to_int(ususp),
    }
    raw = {
        "interactive": str(interactive).lower(),
    }
    if interactive_source:
        raw["interactive_source"] = interactive_source
    if host_spec:
        raw.update({
            "host_spec": host_spec,
            "host_spec_tokens": compact_json(split_host_tokens(host_spec)),
            "host_groups": compact_json(queue_groups),
            "hosts": compact_json(queue_hosts),
        })
    if user_spec:
        raw.update({
            "user_spec": user_spec,
            "user_spec_tokens": compact_json(split_host_tokens(user_spec)),
            "user_groups": compact_json(queue_user_groups),
            "users": compact_json(queue_users),
        })
    queue["raw"] = raw
    queues.append(queue)


licenses = []
for line in run(["blstat"]):
    lower = line.lower()
    if lower.startswith("feature") or lower.startswith("-"):
        continue
    parts = re.split(r"\s+", line)
    if len(parts) < 4:
        continue
    feature = parts[0]
    numbers = [to_float(value) for value in parts[1:] if re.match(r"^-?\d+(\.\d+)?$", value)]
    if len(numbers) >= 2:
        total = numbers[0]
        used = numbers[1]
        licenses.append(
            {
                "feature": feature,
                "total": total,
                "used": used,
                "free": max(total - used, 0.0),
            }
        )

if not licenses and shutil.which("lmstat") is not None:
    for line in run(["lmstat", "-a"]):
        match = re.search(
            r"Users of ([^:]+):.*Total of (\d+) licenses? issued;.*Total of (\d+) licenses? in use",
            line,
        )
        if not match:
            continue
        feature = match.group(1).strip()
        total = to_float(match.group(2))
        used = to_float(match.group(3))
        licenses.append(
            {
                "feature": feature,
                "total": total,
                "used": used,
                "free": max(total - used, 0.0),
            }
        )


custom_resources = []
for group_name, members in sorted(host_groups.items()):
    custom_resources.append(
        {
            "name": group_name,
            "type": "host_group",
            "location": "cluster",
            "total": float(len(members)),
            "raw": {
                "source": "bmgroup -w",
                "members": compact_json(members),
            },
        }
    )

for line in run(["lsinfo", "-r"]):
    lower = line.lower()
    if lower.startswith("resource") or lower.startswith("-"):
        continue
    parts = re.split(r"\s+", line, maxsplit=4)
    if len(parts) < 2:
        continue
    name = parts[0]
    resource_type = parts[1]
    if name in ("-", ""):
        continue
    custom_resources.append(
        {
            "name": name,
            "type": resource_type,
            "location": "cluster",
            "raw": {
                "source": "lsinfo -r",
                "line": line,
            },
        }
    )


payload = {
    "queues": queues,
    "hosts": hosts,
    "cluster": cluster,
    "licenses": licenses,
    "custom_resources": custom_resources,
}

json.dump(payload, sys.stdout, separators=(",", ":"))
sys.stdout.write("\n")
PY
